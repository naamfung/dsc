package subtype

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// Widen converts a type to a more general form by replacing literals with base types.
//
// Widening is used when types flow to mutable locations or when we want to
// prevent overly-specific type inference.
//
// Widening rules:
//   - Literal boolean -> boolean
//   - Literal integer -> integer
//   - Literal number -> number
//   - Literal string -> string
//   - Union: widen each member, then normalize
//   - Optional: widen the inner type
//   - All other types: unchanged
//
// This is a shallow operation; nested types within records, arrays, maps,
// and functions are not widened. Use [WidenForInference] for deep widening.
//
// Returns nil if t is nil.
func Widen(t typ.Type) typ.Type {
	return widenDepth(t, 0)
}

// widenDepth is the recursive implementation of Widen with depth tracking
// to prevent infinite recursion.
func widenDepth(t typ.Type, depth int) typ.Type {
	if stopDepth(t, depth) {
		return t
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Literal: func(lit *typ.Literal) typ.Type {
			switch lit.Base {
			case kind.Boolean:
				return typ.Boolean
			case kind.Integer:
				return typ.Integer
			case kind.Number:
				return typ.Number
			case kind.String:
				return typ.String
			}
			return t
		},
		Union: func(u *typ.Union) typ.Type {
			members := make([]typ.Type, len(u.Members))
			changed := false

			for i, m := range u.Members {
				members[i] = widenDepth(m, depth+1)
				if members[i] != m {
					changed = true
				}
			}

			if !changed {
				return t
			}

			return typ.NewUnion(members...)
		},
		Optional: func(o *typ.Optional) typ.Type {
			inner := widenDepth(o.Inner, depth+1)
			if inner == o.Inner {
				return t
			}

			return typ.NewOptional(inner)
		},
		Default: func(t typ.Type) typ.Type {
			return t
		},
	})
}

// WidenForInference performs deep widening, recursively widening nested types.
//
// Used during type inference to prevent overly specific inferred types.
// This is more aggressive than [Widen] and processes nested structures:
//
//   - Tuple elements: each element is recursively widened
//   - Array elements: the element type is recursively widened
//   - Map key/value: both are recursively widened
//   - Record fields: each field type is recursively widened
//   - Large records (> DefaultRecursionDepth fields): collapsed to Map<string, union>
//
// The large record collapse prevents type explosion when inferring types
// for data structures with many fields.
//
// Returns nil if t is nil.
func WidenForInference(t typ.Type) typ.Type {
	return widenForInferenceDepth(t, 0)
}

// widenForInferenceDepth is the recursive implementation with depth tracking.
func widenForInferenceDepth(t typ.Type, depth int) typ.Type {
	if stopDepth(t, depth) {
		return t
	}

	// First apply normal widening
	t = widenDepth(t, depth)

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Tuple: func(tup *typ.Tuple) typ.Type {
			// Widen tuple elements
			elems := make([]typ.Type, len(tup.Elements))
			changed := false

			for i, e := range tup.Elements {
				elems[i] = widenForInferenceDepth(e, depth+1)
				if elems[i] != e {
					changed = true
				}
			}

			if !changed {
				return t
			}

			return typ.NewTuple(elems...)
		},
		Array: func(a *typ.Array) typ.Type {
			elem := widenForInferenceDepth(a.Element, depth+1)
			if elem == a.Element {
				return t
			}

			return typ.NewArray(elem)
		},
		Map: func(m *typ.Map) typ.Type {
			key := widenForInferenceDepth(m.Key, depth+1)
			val := widenForInferenceDepth(m.Value, depth+1)

			if key == m.Key && val == m.Value {
				return t
			}

			return typ.NewMap(key, val)
		},
		Record: func(r *typ.Record) typ.Type {
			if len(r.Fields) > typ.DefaultRecursionDepth {
				var fieldTypes []typ.Type
				for _, f := range r.Fields {
					fieldTypes = append(fieldTypes, widenForInferenceDepth(f.Type, depth+1))
				}

				elem := typ.Unknown
				if len(fieldTypes) > 0 {
					elem = typ.NewUnion(fieldTypes...)
				}

				return typ.NewMap(typ.String, elem)
			}

			builder := typ.NewRecord()
			if r.Open {
				builder.SetOpen(true)
			}

			for _, f := range r.Fields {
				fieldType := widenForInferenceDepth(f.Type, depth+1)

				switch {
				case f.Optional && f.Readonly:
					builder.OptReadonlyField(f.Name, fieldType)
				case f.Optional:
					builder.OptField(f.Name, fieldType)
				case f.Readonly:
					builder.ReadonlyField(f.Name, fieldType)
				default:
					builder.Field(f.Name, fieldType)
				}
			}

			if r.Metatable != nil {
				builder.Metatable(widenForInferenceDepth(r.Metatable, depth+1))
			}

			if r.HasMapComponent() {
				builder.MapComponent(
					widenForInferenceDepth(r.MapKey, depth+1),
					widenForInferenceDepth(r.MapValue, depth+1),
				)
			}

			return builder.Build()
		},
		Function: func(fn *typ.Function) typ.Type {
			// Preserve generic signatures as-is to avoid detaching type-param
			// references from their declaration list.
			if len(fn.TypeParams) > 0 {
				return t
			}

			params := make([]typ.Param, len(fn.Params))
			changed := false
			for i, p := range fn.Params {
				pt := widenForInferenceDepth(p.Type, depth+1)
				params[i] = typ.Param{
					Name:     p.Name,
					Type:     pt,
					Optional: p.Optional,
				}
				if pt != p.Type {
					changed = true
				}
			}

			var variadic typ.Type
			if fn.Variadic != nil {
				variadic = widenForInferenceDepth(fn.Variadic, depth+1)
				if variadic != fn.Variadic {
					changed = true
				}
			}

			rets := make([]typ.Type, len(fn.Returns))
			for i, ret := range fn.Returns {
				rt := widenForInferenceDepth(ret, depth+1)
				rets[i] = rt
				if rt != ret {
					changed = true
				}
			}

			if !changed {
				return t
			}

			builder := typ.Func().
				Effects(fn.Effects).
				Spec(fn.Spec).
				WithRefinement(fn.Refinement)
			for _, p := range params {
				if p.Optional {
					builder = builder.OptParam(p.Name, p.Type)
				} else {
					builder = builder.Param(p.Name, p.Type)
				}
			}
			if variadic != nil {
				builder = builder.Variadic(variadic)
			}
			builder = builder.Returns(rets...)
			return builder.Build()
		},
		Interface: func(in *typ.Interface) typ.Type {
			changed := false
			methods := make([]typ.Method, len(in.Methods))
			for i, m := range in.Methods {
				mt := widenForInferenceDepth(m.Type, depth+1)
				methodFn, ok := mt.(*typ.Function)
				if !ok {
					methodFn = m.Type
				}
				methods[i] = typ.Method{Name: m.Name, Type: methodFn}
				if methodFn != m.Type {
					changed = true
				}
			}
			if !changed {
				return t
			}
			return typ.NewInterface(in.Name, methods)
		},
		Default: func(t typ.Type) typ.Type {
			return t
		},
	})
}
