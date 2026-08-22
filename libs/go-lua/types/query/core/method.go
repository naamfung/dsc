package core

import (
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Method resolves a method type on a container without using an engine.
//
// This is the pure, non-memoized version of method lookup. Methods in Lua
// are accessed via colon syntax (obj:method()) which passes obj as the first
// argument. This function resolves the method's type signature.
//
// Search order:
//  1. Meta type's built-in :is method
//  2. Record fields that are function types
//  3. Interface method signatures
//  4. Metatable fields for records with metatables
//  5. __index chain for inherited methods
//
// For unions, the method must exist with compatible signatures in all non-nil
// members. Returns (nil, false) if the method does not exist.
func Method(t typ.Type, name string) (typ.Type, bool) {
	return methodDepth(t, name, 0)
}

// methodViaIndex walks the __index chain to find inherited methods.
//
// Lua's prototype inheritance uses __index to delegate lookups to parent tables.
// This function follows the chain:
//  1. Get __index field from current metatable
//  2. If __index is a table, look up the method there and recurse
//  3. If __index is a function, use its return type as the method type
func methodViaIndex(meta typ.Type, name string, depth int) (typ.Type, bool) {
	if stopDepth(meta, depth) {
		return nil, false
	}

	rec, ok := meta.(*typ.Record)
	if !ok {
		return nil, false
	}

	// Get __index from this metatable
	indexField := rec.GetField("__index")
	if indexField == nil {
		return nil, false
	}

	res := typ.Visit(indexField.Type, typ.Visitor[fieldResult]{
		Record: func(r *typ.Record) fieldResult {
			// __index is a table - look for method there
			if ft, ok := fieldDepth(r, name, depth+1); ok {
				return fieldResult{t: ft, ok: true}
			}
			// Continue walking the chain
			if r.Metatable != nil {
				ft, ok := methodViaIndex(r.Metatable, name, depth+1)
				return fieldResult{t: ft, ok: ok}
			}

			return fieldResult{}
		},
		Function: func(fn *typ.Function) fieldResult {
			// __index is a function - returns any
			if len(fn.Returns) > 0 {
				return fieldResult{t: fn.Returns[0], ok: true}
			}

			return fieldResult{t: typ.Unknown, ok: true}
		},
		Default: func(t typ.Type) fieldResult {
			return fieldResult{}
		},
	})
	return res.t, res.ok
}

// methodDepth recursively resolves method lookup with depth limiting.
// Handles various type constructors and propagates through wrappers.
func methodDepth(t typ.Type, name string, depth int) (typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}
	if top, ok := specialAccessType(t); ok {
		return top, true
	}

	res := typ.Visit(t, typ.Visitor[fieldResult]{
		TypeParam: func(tp *typ.TypeParam) fieldResult {
			if tp.Constraint != nil {
				mt, ok := methodDepth(tp.Constraint, name, depth+1)
				return fieldResult{t: mt, ok: ok}
			}
			return fieldResult{}
		},
		Meta: func(m *typ.Meta) fieldResult {
			// Meta types have a built-in :is method for type guards
			if name == "is" {
				return fieldResult{t: metaIsMethod(m.Of), ok: true}
			}
			return fieldResult{}
		},
		Record: func(r *typ.Record) fieldResult {
			// Check if record has a field with function type (can be called as method)
			if ft, ok := fieldDepth(r, name, depth+1); ok {
				if unwrap.Function(ft) != nil {
					return fieldResult{t: ft, ok: true}
				}
			}

			if r.Metatable == nil {
				if r.Open {
					// Open record: allow unknown methods to be called.
					return fieldResult{t: typ.Func().Variadic(typ.Any).Returns(typ.Any).Build(), ok: true}
				}
				return fieldResult{}
			}
			// Look for method in metatable's fields
			if ft, ok := fieldDepth(r.Metatable, name, depth+1); ok {
				return fieldResult{t: ft, ok: true}
			}
			// Check __index chain for inherited methods
			if mt, ok := methodViaIndex(r.Metatable, name, depth+1); ok {
				return fieldResult{t: mt, ok: true}
			}
			if r.Open {
				// Open record: allow unknown methods even when metatable has no method.
				return fieldResult{t: typ.Func().Variadic(typ.Any).Returns(typ.Any).Build(), ok: true}
			}
			return fieldResult{}
		},
		Interface: func(i *typ.Interface) fieldResult {
			for _, m := range i.Methods {
				if m.Name == name {
					return fieldResult{t: m.Type, ok: true}
				}
			}

			return fieldResult{}
		},
		Function: func(fn *typ.Function) fieldResult {
			return fieldResult{}
		},
		Union: func(u *typ.Union) fieldResult {
			var result typ.Type

			hasNil := false

			for _, m := range u.Members {
				// Skip nil in union - nil | T should allow method calls on T
				if m.Kind() == kind.Nil {
					hasNil = true
					continue
				}

				mt, ok := methodDepth(m, name, depth+1)
				if !ok {
					return fieldResult{}
				}

				if result == nil {
					result = mt
				} else if !result.Equals(mt) {
					return fieldResult{}
				}
			}
			// If union was only nil members, no method found
			if result == nil && hasNil {
				return fieldResult{}
			}

			return fieldResult{t: result, ok: result != nil}
		},
		Intersection: func(in *typ.Intersection) fieldResult {
			for _, m := range in.Members {
				if mt, ok := methodDepth(m, name, depth+1); ok {
					return fieldResult{t: mt, ok: true}
				}
			}

			return fieldResult{}
		},
		Optional: func(o *typ.Optional) fieldResult {
			mt, ok := methodDepth(o.Inner, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Recursive: func(rec *typ.Recursive) fieldResult {
			if rec.Body == nil || rec.Body == rec {
				return fieldResult{}
			}
			mt, ok := methodDepth(rec.Body, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Alias: func(a *typ.Alias) fieldResult {
			mt, ok := methodDepth(a.Target, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Instantiated: func(inst *typ.Instantiated) fieldResult {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return fieldResult{}
			}

			mt, ok := methodDepth(resolved, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Ref: func(r *typ.Ref) fieldResult {
			return fieldResult{}
		},
		Default: func(t typ.Type) fieldResult {
			return fieldResult{}
		},
	})
	return res.t, res.ok
}

// HasMethod returns true if a type has a method with the given name.
//
// This is a convenience wrapper around Method that discards the type result.
func HasMethod(t typ.Type, name string) bool {
	_, ok := methodDepth(t, name, 0)
	return ok
}

// FieldOrMethod looks up a name as either field or method.
//
// This combines field and method lookup, preferring fields. It handles the
// ambiguity in Lua where t.name could access a field or a method (methods
// are just fields containing functions).
//
// Returns (nil, false) if neither field nor method exists.
func FieldOrMethod(t typ.Type, name string) (typ.Type, bool) {
	if ft, ok := fieldDepth(t, name, 0); ok {
		return ft, true
	}

	return methodDepth(t, name, 0)
}

// Callable returns the function type if t is callable.
//
// A type is callable if it can be invoked as a function. This includes:
//   - Function types directly
//   - Records with __call metamethod
//   - Unions where all members are callable
//   - Intersections where any member is callable
//   - Generic types whose body is callable
//
// Returns (nil, false) if the type cannot be called.
func Callable(t typ.Type) (*typ.Function, bool) {
	return callableDepth(t, 0)
}

// callableDepth recursively checks callability with depth limiting.
func callableDepth(t typ.Type, depth int) (*typ.Function, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}

	res := typ.Visit(t, typ.Visitor[callableResult]{
		Function: func(fn *typ.Function) callableResult {
			return callableResult{fn: fn, ok: true}
		},
		Optional: func(o *typ.Optional) callableResult {
			fn, ok := callableDepth(o.Inner, depth+1)
			return callableResult{fn: fn, ok: ok}
		},
		Recursive: func(rec *typ.Recursive) callableResult {
			if rec.Body == nil || rec.Body == rec {
				return callableResult{}
			}
			fn, ok := callableDepth(rec.Body, depth+1)
			return callableResult{fn: fn, ok: ok}
		},
		Alias: func(a *typ.Alias) callableResult {
			fn, ok := callableDepth(a.Target, depth+1)
			return callableResult{fn: fn, ok: ok}
		},
		Instantiated: func(inst *typ.Instantiated) callableResult {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return callableResult{}
			}

			fn, ok := callableDepth(resolved, depth+1)
			return callableResult{fn: fn, ok: ok}
		},
		Generic: func(g *typ.Generic) callableResult {
			fn, ok := callableDepth(g.Body, depth+1)
			return callableResult{fn: fn, ok: ok}
		},
		Union: func(u *typ.Union) callableResult {
			var result *typ.Function

			for _, m := range u.Members {
				f, ok := callableDepth(m, depth+1)
				if !ok {
					return callableResult{}
				}

				if result == nil {
					result = f
				}
			}

			return callableResult{fn: result, ok: result != nil}
		},
		Intersection: func(in *typ.Intersection) callableResult {
			for _, m := range in.Members {
				if f, ok := callableDepth(m, depth+1); ok {
					return callableResult{fn: f, ok: true}
				}
			}

			return callableResult{}
		},
		Record: func(r *typ.Record) callableResult {
			// Check for __call metamethod
			if r.Metatable == nil {
				return callableResult{}
			}

			callMeta, ok := fieldDepth(r.Metatable, "__call", depth+1)
			if !ok {
				return callableResult{}
			}

			if fn, ok := callMeta.(*typ.Function); ok {
				return callableResult{fn: fn, ok: true}
			}

			return callableResult{}
		},
		Default: func(t typ.Type) callableResult {
			return callableResult{}
		},
	})
	return res.fn, res.ok
}

// GetMetamethod returns the type of a metamethod if present.
//
// Metamethods are special fields in a type's metatable that customize behavior:
//   - __index: field access fallback
//   - __newindex: field assignment fallback
//   - __call: function call operator
//   - __add, __sub, __mul, etc.: arithmetic operators
//   - __eq, __lt, __le: comparison operators
//   - __tostring: string conversion
//   - __len: length operator
//
// Returns (nil, false) if the type has no metatable or lacks the metamethod.
func GetMetamethod(t typ.Type, name string) (typ.Type, bool) {
	return getMetamethodDepth(t, name, 0)
}

// getMetamethodDepth recursively looks up metamethods with depth limiting.
func getMetamethodDepth(t typ.Type, name string, depth int) (typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}

	res := typ.Visit(t, typ.Visitor[fieldResult]{
		Record: func(r *typ.Record) fieldResult {
			if r.Metatable == nil {
				return fieldResult{}
			}

			mt, ok := fieldDepth(r.Metatable, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Optional: func(o *typ.Optional) fieldResult {
			mt, ok := getMetamethodDepth(o.Inner, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Recursive: func(rec *typ.Recursive) fieldResult {
			if rec.Body == nil || rec.Body == rec {
				return fieldResult{}
			}
			mt, ok := getMetamethodDepth(rec.Body, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Alias: func(a *typ.Alias) fieldResult {
			mt, ok := getMetamethodDepth(a.Target, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Instantiated: func(inst *typ.Instantiated) fieldResult {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return fieldResult{}
			}

			mt, ok := getMetamethodDepth(resolved, name, depth+1)
			return fieldResult{t: mt, ok: ok}
		},
		Default: func(t typ.Type) fieldResult {
			return fieldResult{}
		},
	})
	return res.t, res.ok
}

// HasMetamethod returns true if the type has the specified metamethod.
//
// This is a convenience wrapper around GetMetamethod that discards the type.
func HasMetamethod(t typ.Type, name string) bool {
	_, ok := getMetamethodDepth(t, name, 0)
	return ok
}

// metaIsMethod returns the built-in :is method signature for Meta types.
//
// Meta types (reified type values) have a built-in :is method for runtime
// type guards: value:is(Type) returns (value|nil, err?).
// Signature: (value: any) -> (T?, LuaError?)
func metaIsMethod(of typ.Type) *typ.Function {
	if of == nil {
		of = typ.Any
	}
	return typ.Func().
		Param("value", typ.Any).
		Returns(typ.NewOptional(of), typ.NewOptional(typ.LuaError)).
		Effects(effect.WithTypeValueMethod()).
		Build()
}
