package core

import (
	"errors"

	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// Errors returned by generic instantiation functions.
var (
	// ErrNotGeneric indicates an attempt to instantiate a non-generic type.
	ErrNotGeneric = errors.New("type is not generic")

	// ErrTypeArgCount indicates a mismatch between type parameters and arguments.
	ErrTypeArgCount = errors.New("wrong number of type arguments")

	// ErrConstraintViolation indicates a type argument that doesn't satisfy its constraint.
	ErrConstraintViolation = errors.New("type argument violates constraint")
)

// InstantiateGeneric substitutes type arguments into a generic type body.
//
// This is the core generic instantiation function. Given a generic type
// definition and concrete type arguments, it:
//  1. Validates the argument count matches the parameter count
//  2. Validates each argument satisfies its parameter's constraint
//  3. Builds a substitution map from parameter names to argument types
//  4. Recursively substitutes parameters throughout the body
//
// Example:
//
//	// Given: type List<T> = {items: T[]}
//	// Call:  InstantiateGeneric(List, [number])
//	// Result: {items: number[]}
//
// Returns an error if the generic is nil, argument count is wrong, or any
// argument violates its constraint.
func InstantiateGeneric(g *typ.Generic, typeArgs []typ.Type) (typ.Type, error) {
	if g == nil {
		return nil, ErrNotGeneric
	}

	if len(typeArgs) != len(g.TypeParams) {
		return nil, ErrTypeArgCount
	}

	// Validate constraints
	for i, arg := range typeArgs {
		constraint := g.TypeParams[i].Constraint
		if constraint != nil && !subtype.IsSubtype(arg, constraint) {
			return nil, ErrConstraintViolation
		}
	}

	// Build substitution map
	subst := make(map[string]typ.Type, len(g.TypeParams))
	for i, p := range g.TypeParams {
		subst[p.Name] = typeArgs[i]
	}

	return Substitute(g.Body, subst), nil
}

// Substitute replaces type parameters with concrete types throughout a type.
//
// This performs structural substitution: every TypeParam node whose name appears
// in the subst map is replaced with the corresponding concrete type. The
// substitution is recursive and handles all composite types:
//
//   - Functions: substitutes in parameters, variadic, and returns
//   - Records: substitutes in field types and metatable
//   - Interfaces: substitutes in method types
//   - Arrays, Maps, Tuples: substitutes in element types
//   - Unions, Intersections: substitutes in member types
//
// Cycle detection via visited map prevents infinite recursion on recursive types.
// Nested generic definitions are NOT substituted (they have their own scope).
func Substitute(t typ.Type, subst map[string]typ.Type) typ.Type {
	return substituteVisited(t, subst, make(map[typ.Type]typ.Type))
}

// substituteVisited performs substitution with cycle detection.
// The visited map tracks already-processed types to handle recursive definitions.
// For cycle handling, types are pre-registered in visited before processing.
func substituteVisited(t typ.Type, subst map[string]typ.Type, visited map[typ.Type]typ.Type) typ.Type {
	if t == nil {
		return nil
	}

	// Check cache for cycles
	if result, ok := visited[t]; ok {
		return result
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		TypeParam: func(tp *typ.TypeParam) typ.Type {
			if replacement, ok := subst[tp.Name]; ok {
				return replacement
			}

			return t
		},
		Function: func(fn *typ.Function) typ.Type {
			// Pre-register to handle cycles
			visited[t] = t

			newParams := make([]typ.Param, len(fn.Params))
			changed := false

			for i, p := range fn.Params {
				newType := substituteVisited(p.Type, subst, visited)
				if newType != p.Type {
					changed = true
				}

				newParams[i] = typ.Param{Name: p.Name, Type: newType, Optional: p.Optional}
			}

			var newVariadic typ.Type
			if fn.Variadic != nil {
				newVariadic = substituteVisited(fn.Variadic, subst, visited)
				if newVariadic != fn.Variadic {
					changed = true
				}
			}

			newReturns := make([]typ.Type, len(fn.Returns))
			for i, r := range fn.Returns {
				newReturns[i] = substituteVisited(r, subst, visited)
				if newReturns[i] != r {
					changed = true
				}
			}

			if !changed {
				return t
			}

			builder := typ.Func()

			for _, p := range newParams {
				if p.Optional {
					builder.OptParam(p.Name, p.Type)
				} else {
					builder.Param(p.Name, p.Type)
				}
			}

			if newVariadic != nil {
				builder.Variadic(newVariadic)
			}

			builder.Returns(newReturns...)
			if fn.Effects != nil {
				builder.Effects(fn.Effects)
			}
			if fn.Spec != nil {
				builder.Spec(fn.Spec)
			}
			if fn.Refinement != nil {
				builder.WithRefinement(fn.Refinement)
			}
			result := builder.Build()
			visited[t] = result

			return result
		},
		Record: func(r *typ.Record) typ.Type {
			visited[t] = t

			newFields := make([]typ.Field, len(r.Fields))
			changed := false

			for i, f := range r.Fields {
				newType := substituteVisited(f.Type, subst, visited)
				if newType != f.Type {
					changed = true
				}

				newFields[i] = typ.Field{
					Name:     f.Name,
					Type:     newType,
					Optional: f.Optional,
					Readonly: f.Readonly,
				}
			}

			if !changed {
				return t
			}

			builder := typ.NewRecord()

			for _, f := range newFields {
				if f.Optional {
					builder.OptField(f.Name, f.Type)
				} else if f.Readonly {
					builder.ReadonlyField(f.Name, f.Type)
				} else {
					builder.Field(f.Name, f.Type)
				}
			}

			if r.Metatable != nil {
				newMeta := substituteVisited(r.Metatable, subst, visited)
				builder.Metatable(newMeta)
			}

			result := builder.Build()
			visited[t] = result

			return result
		},
		Interface: func(i *typ.Interface) typ.Type {
			visited[t] = t

			newMethods := make([]typ.Method, len(i.Methods))
			changed := false

			for idx, m := range i.Methods {
				newType := substituteVisited(m.Type, subst, visited)

				fn, ok := newType.(*typ.Function)
				if !ok {
					fn = m.Type
				}

				if fn != m.Type {
					changed = true
				}

				newMethods[idx] = typ.Method{Name: m.Name, Type: fn}
			}

			if !changed {
				return t
			}

			result := typ.NewInterface(i.Name, newMethods)
			visited[t] = result

			return result
		},
		Array: func(a *typ.Array) typ.Type {
			newElem := substituteVisited(a.Element, subst, visited)
			if newElem == a.Element {
				return t
			}

			return typ.NewArray(newElem)
		},
		Map: func(m *typ.Map) typ.Type {
			newKey := substituteVisited(m.Key, subst, visited)
			newValue := substituteVisited(m.Value, subst, visited)

			if newKey == m.Key && newValue == m.Value {
				return t
			}

			return typ.NewMap(newKey, newValue)
		},
		Tuple: func(tup *typ.Tuple) typ.Type {
			newElems := make([]typ.Type, len(tup.Elements))
			changed := false

			for i, e := range tup.Elements {
				newElems[i] = substituteVisited(e, subst, visited)
				if newElems[i] != e {
					changed = true
				}
			}

			if !changed {
				return t
			}

			return typ.NewTuple(newElems...)
		},
		Optional: func(o *typ.Optional) typ.Type {
			newInner := substituteVisited(o.Inner, subst, visited)
			if newInner == o.Inner {
				return t
			}

			return typ.NewOptional(newInner)
		},
		Union: func(u *typ.Union) typ.Type {
			newMembers := make([]typ.Type, len(u.Members))
			changed := false

			for i, m := range u.Members {
				newMembers[i] = substituteVisited(m, subst, visited)
				if newMembers[i] != m {
					changed = true
				}
			}

			if !changed {
				return t
			}

			return typ.NewUnion(newMembers...)
		},
		Intersection: func(in *typ.Intersection) typ.Type {
			newMembers := make([]typ.Type, len(in.Members))
			changed := false

			for i, m := range in.Members {
				newMembers[i] = substituteVisited(m, subst, visited)
				if newMembers[i] != m {
					changed = true
				}
			}

			if !changed {
				return t
			}

			return typ.NewIntersection(newMembers...)
		},
		Ref: func(r *typ.Ref) typ.Type {
			// Refs are unresolved name references - no substitution needed
			return t
		},
		Alias: func(a *typ.Alias) typ.Type {
			newTarget := substituteVisited(a.Target, subst, visited)
			if newTarget == a.Target {
				return t
			}

			return typ.NewAlias(a.Name, newTarget)
		},
		Generic: func(g *typ.Generic) typ.Type {
			// Don't substitute inside nested generic definitions
			// (they have their own scope for type params)
			return t
		},
		Instantiated: func(inst *typ.Instantiated) typ.Type {
			// Substitute in type args
			newArgs := make([]typ.Type, len(inst.TypeArgs))
			changed := false

			for i, a := range inst.TypeArgs {
				newArgs[i] = substituteVisited(a, subst, visited)
				if newArgs[i] != a {
					changed = true
				}
			}

			if !changed {
				return t
			}

			return typ.Instantiate(inst.Generic, newArgs...)
		},
		Default: func(t typ.Type) typ.Type {
			// Primitives, literals, etc. - no substitution needed
			return t
		},
	})
}

// ResolveInstantiated fully resolves an Instantiated type to its body.
//
// An Instantiated type represents a generic type with type arguments already
// bound (e.g., List<number>). This function expands it to the concrete type
// by substituting the arguments into the generic body.
//
// This is a convenience wrapper around InstantiateGeneric that extracts the
// generic and type arguments from the Instantiated node.
func ResolveInstantiated(inst *typ.Instantiated) (typ.Type, error) {
	return InstantiateGeneric(inst.Generic, inst.TypeArgs)
}

// CollectTypeParams returns all type parameters found in a type.
//
// This traverses the type structure and collects all TypeParam nodes,
// which is useful for:
//   - Determining if a type is fully concrete or still parameterized
//   - Identifying which parameters need inference during type checking
//   - Validating that all parameters are bound before instantiation
//
// Returns an empty slice if the type contains no type parameters.
func CollectTypeParams(t typ.Type) []*typ.TypeParam {
	var params []*typ.TypeParam

	collectTypeParamsVisited(t, &params, make(map[typ.Type]bool))

	return params
}

// collectTypeParamsVisited recursively collects type parameters with cycle detection.
func collectTypeParamsVisited(t typ.Type, params *[]*typ.TypeParam, visited map[typ.Type]bool) {
	if t == nil || visited[t] {
		return
	}

	visited[t] = true

	typ.Visit(t, typ.Visitor[struct{}]{
		TypeParam: func(tp *typ.TypeParam) struct{} {
			*params = append(*params, tp)
			return struct{}{}
		},
		Function: func(fn *typ.Function) struct{} {
			for _, p := range fn.Params {
				collectTypeParamsVisited(p.Type, params, visited)
			}

			collectTypeParamsVisited(fn.Variadic, params, visited)

			for _, r := range fn.Returns {
				collectTypeParamsVisited(r, params, visited)
			}
			return struct{}{}
		},
		Record: func(r *typ.Record) struct{} {
			for _, f := range r.Fields {
				collectTypeParamsVisited(f.Type, params, visited)
			}
			return struct{}{}
		},
		Array: func(a *typ.Array) struct{} {
			collectTypeParamsVisited(a.Element, params, visited)
			return struct{}{}
		},
		Map: func(m *typ.Map) struct{} {
			collectTypeParamsVisited(m.Key, params, visited)
			collectTypeParamsVisited(m.Value, params, visited)
			return struct{}{}
		},
		Tuple: func(tup *typ.Tuple) struct{} {
			for _, e := range tup.Elements {
				collectTypeParamsVisited(e, params, visited)
			}
			return struct{}{}
		},
		Optional: func(o *typ.Optional) struct{} {
			collectTypeParamsVisited(o.Inner, params, visited)
			return struct{}{}
		},
		Union: func(u *typ.Union) struct{} {
			for _, m := range u.Members {
				collectTypeParamsVisited(m, params, visited)
			}
			return struct{}{}
		},
		Intersection: func(in *typ.Intersection) struct{} {
			for _, m := range in.Members {
				collectTypeParamsVisited(m, params, visited)
			}
			return struct{}{}
		},
		Ref: func(r *typ.Ref) struct{} {
			// Refs are unresolved name references - no type params inside
			return struct{}{}
		},
		Alias: func(a *typ.Alias) struct{} {
			collectTypeParamsVisited(a.Target, params, visited)
			return struct{}{}
		},
		Instantiated: func(inst *typ.Instantiated) struct{} {
			for _, a := range inst.TypeArgs {
				collectTypeParamsVisited(a, params, visited)
			}
			return struct{}{}
		},
		Default: func(t typ.Type) struct{} {
			return struct{}{}
		},
	})
}

// HasTypeParams returns true if the type contains any type parameters.
//
// A type with parameters is not fully concrete and requires instantiation
// before it can be used for runtime values. This is a quick check that
// avoids building the full parameter list when only presence is needed.
func HasTypeParams(t typ.Type) bool {
	return len(CollectTypeParams(t)) > 0
}
