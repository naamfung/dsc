// Package subst provides type substitution operations for generics.
//
// These operations replace type parameters with concrete types,
// used during generic instantiation and Self type resolution.
package subst

import (
	"sync"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// Substitute replaces type parameters with concrete types throughout a type.
//
// Used during generic instantiation to replace TypeParam references with
// the corresponding type arguments. The subs map keys are type parameter names.
func Substitute(t typ.Type, subs map[string]typ.Type) typ.Type {
	if len(subs) == 0 {
		return t
	}
	return typ.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if tp, ok := n.(*typ.TypeParam); ok {
			if sub, ok := subs[tp.Name]; ok {
				return sub, true
			}
		}
		return nil, false
	})
}

// Params replaces type parameters with corresponding type arguments.
func Params(t typ.Type, params []*typ.TypeParam, args []typ.Type) typ.Type {
	if len(params) != len(args) || len(params) == 0 {
		return t
	}
	subs := make(map[string]typ.Type, len(params))
	for i, p := range params {
		subs[p.Name] = args[i]
	}
	return Substitute(t, subs)
}

// Self replaces Self type references with a concrete type.
// Does not recurse into Interface types because Self inside an Interface
// is a separate binding that refers to that Interface's implementor.
func Self(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	return typ.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if n.Kind() == kind.Self {
			return selfType, true
		}
		if _, ok := n.(*typ.Interface); ok {
			return n, true
		}
		return nil, false
	})
}

// ExpandInstantiated expands generic instantiations to their structural form.
//
// For Instantiated{Generic: Array<T>, TypeArgs: [number]}, returns the Array
// body with T replaced by number. This enables structural comparison and
// field/method lookup on instantiated generics.
//
// Does not enforce generic constraints; use subtype checking for that.
func ExpandInstantiated(t typ.Type) typ.Type {
	if t == nil || !expandInstantiatedCanDescend(t) {
		return t
	}
	memo := getExpandMemo()
	defer putExpandMemo(memo)
	guard := typ.GuardForDepth(typ.DefaultRecursionDepth)
	return expandInstantiatedGuard(t, guard, memo)
}

const expandMemoMaxEntries = 2048

var expandMemoPool = sync.Pool{
	New: func() any {
		return make(map[typ.Type]typ.Type, 32)
	},
}

func getExpandMemo() map[typ.Type]typ.Type {
	return expandMemoPool.Get().(map[typ.Type]typ.Type)
}

func putExpandMemo(m map[typ.Type]typ.Type) {
	if len(m) > expandMemoMaxEntries {
		expandMemoPool.Put(make(map[typ.Type]typ.Type, 32))
		return
	}
	clear(m)
	expandMemoPool.Put(m)
}

func expandInstantiatedGuard(t typ.Type, guard internal.RecursionGuard, memo map[typ.Type]typ.Type) typ.Type {
	if t == nil || !expandInstantiatedCanDescend(t) {
		return t
	}

	if cached, ok := memo[t]; ok {
		return cached
	}

	next, ok := guard.Enter(t)
	if !ok {
		return t
	}

	orig := t
	for {
		ann, ok := t.(*typ.Annotated)
		if !ok || ann.Inner == nil || ann.Inner == t {
			break
		}
		t = ann.Inner
	}

	result := expandInstantiatedCore(t, orig, next, memo)
	memo[orig] = result
	return result
}

func expandInstantiatedCore(t typ.Type, orig typ.Type, guard internal.RecursionGuard, memo map[typ.Type]typ.Type) typ.Type {
	switch v := t.(type) {
	case *typ.Instantiated:
		if v.Generic == nil || len(v.TypeArgs) != len(v.Generic.TypeParams) || v.Generic.Body == nil {
			return orig
		}
		body := Params(v.Generic.Body, v.Generic.TypeParams, v.TypeArgs)
		body = Self(body, orig)
		return expandInstantiatedGuard(body, guard, memo)
	case *typ.Optional:
		inner := expandInstantiatedGuard(v.Inner, guard, memo)
		if inner == v.Inner {
			return orig
		}
		return typ.NewOptional(inner)
	case *typ.Union:
		var members []typ.Type
		for i, m := range v.Members {
			newMember := expandInstantiatedGuard(m, guard, memo)
			if newMember != m {
				if members == nil {
					members = make([]typ.Type, len(v.Members))
					copy(members, v.Members)
				}
				members[i] = newMember
			} else if members != nil {
				members[i] = m
			}
		}
		if members == nil {
			return orig
		}
		return typ.NewUnion(members...)
	case *typ.Intersection:
		var members []typ.Type
		for i, m := range v.Members {
			newMember := expandInstantiatedGuard(m, guard, memo)
			if newMember != m {
				if members == nil {
					members = make([]typ.Type, len(v.Members))
					copy(members, v.Members)
				}
				members[i] = newMember
			} else if members != nil {
				members[i] = m
			}
		}
		if members == nil {
			return orig
		}
		return typ.NewIntersection(members...)
	case *typ.Array:
		elem := expandInstantiatedGuard(v.Element, guard, memo)
		if elem == v.Element {
			return orig
		}
		return typ.NewArray(elem)
	case *typ.Map:
		key := expandInstantiatedGuard(v.Key, guard, memo)
		value := expandInstantiatedGuard(v.Value, guard, memo)
		if key == v.Key && value == v.Value {
			return orig
		}
		return typ.NewMap(key, value)
	case *typ.Tuple:
		var elems []typ.Type
		for i, e := range v.Elements {
			newElem := expandInstantiatedGuard(e, guard, memo)
			if newElem != e {
				if elems == nil {
					elems = make([]typ.Type, len(v.Elements))
					copy(elems, v.Elements)
				}
				elems[i] = newElem
			} else if elems != nil {
				elems[i] = e
			}
		}
		if elems == nil {
			return orig
		}
		return typ.NewTuple(elems...)
	case *typ.Function:
		changed := false
		var params []typ.Param
		for i, p := range v.Params {
			newType := p.Type
			if _, isInst := p.Type.(*typ.Instantiated); !isInst {
				newType = expandInstantiatedGuard(p.Type, guard, memo)
			}
			if newType != p.Type {
				if params == nil {
					params = make([]typ.Param, len(v.Params))
					copy(params, v.Params)
				}
				changed = true
				params[i] = typ.Param{Name: p.Name, Type: newType, Optional: p.Optional}
			} else if params != nil {
				params[i] = p
			}
		}

		var returns []typ.Type
		for i, r := range v.Returns {
			newRet := expandInstantiatedGuard(r, guard, memo)
			if newRet != r {
				if returns == nil {
					returns = make([]typ.Type, len(v.Returns))
					copy(returns, v.Returns)
				}
				changed = true
				returns[i] = newRet
			} else if returns != nil {
				returns[i] = r
			}
		}

		variadic := v.Variadic
		if v.Variadic != nil {
			newVariadic := expandInstantiatedGuard(v.Variadic, guard, memo)
			if newVariadic != v.Variadic {
				changed = true
				variadic = newVariadic
			}
		}

		if !changed {
			return orig
		}

		builder := typ.Func()
		paramsSrc := v.Params
		if params != nil {
			paramsSrc = params
		}
		for _, p := range paramsSrc {
			if p.Optional {
				builder = builder.OptParam(p.Name, p.Type)
			} else {
				builder = builder.Param(p.Name, p.Type)
			}
		}
		if variadic != nil {
			builder = builder.Variadic(variadic)
		}
		returnsSrc := v.Returns
		if returns != nil {
			returnsSrc = returns
		}
		if len(returnsSrc) > 0 {
			builder = builder.Returns(returnsSrc...)
		}
		if v.Effects != nil {
			builder = builder.Effects(v.Effects)
		}
		if v.Spec != nil {
			builder = builder.Spec(v.Spec)
		}
		if v.Refinement != nil {
			builder = builder.WithRefinement(v.Refinement)
		}
		return builder.Build()
	case *typ.Record:
		changed := false
		var fields []typ.Field
		for i, f := range v.Fields {
			newType := expandInstantiatedGuard(f.Type, guard, memo)
			if newType != f.Type {
				if fields == nil {
					fields = make([]typ.Field, len(v.Fields))
					copy(fields, v.Fields)
				}
				changed = true
				fields[i] = typ.Field{Name: f.Name, Type: newType, Optional: f.Optional, Readonly: f.Readonly}
			} else if fields != nil {
				fields[i] = f
			}
		}

		metatable := v.Metatable
		if v.Metatable != nil {
			newMetatable := expandInstantiatedGuard(v.Metatable, guard, memo)
			if newMetatable != v.Metatable {
				changed = true
				metatable = newMetatable
			}
		}

		mapKey := v.MapKey
		mapValue := v.MapValue
		if v.HasMapComponent() {
			mapKey = expandInstantiatedGuard(v.MapKey, guard, memo)
			if mapKey != v.MapKey {
				changed = true
			}
			mapValue = expandInstantiatedGuard(v.MapValue, guard, memo)
			if mapValue != v.MapValue {
				changed = true
			}
		}

		if !changed {
			return orig
		}

		builder := typ.NewRecord()
		if v.Open {
			builder.SetOpen(true)
		}
		fieldsSrc := v.Fields
		if fields != nil {
			fieldsSrc = fields
		}
		for _, f := range fieldsSrc {
			switch {
			case f.Optional && f.Readonly:
				builder = builder.OptReadonlyField(f.Name, f.Type)
			case f.Optional:
				builder = builder.OptField(f.Name, f.Type)
			case f.Readonly:
				builder = builder.ReadonlyField(f.Name, f.Type)
			default:
				builder = builder.Field(f.Name, f.Type)
			}
		}
		if metatable != nil {
			builder = builder.Metatable(metatable)
		}
		if mapKey != nil && mapValue != nil {
			builder = builder.MapComponent(mapKey, mapValue)
		}
		return builder.Build()
	case *typ.Alias:
		target := expandInstantiatedGuard(v.Target, guard, memo)
		if target == v.Target {
			return orig
		}
		return typ.NewAlias(v.Name, target)
	case *typ.Interface:
		changed := false
		var methods []typ.Method
		for idx := range v.Methods {
			m := v.Methods[idx]
			newType := expandInstantiatedGuard(m.Type, guard, memo)
			fn, ok := newType.(*typ.Function)
			if !ok {
				fn = m.Type
			}
			if fn != m.Type {
				if methods == nil {
					methods = make([]typ.Method, len(v.Methods))
					copy(methods, v.Methods)
				}
				changed = true
				methods[idx] = typ.Method{Name: m.Name, Type: fn}
			} else if methods != nil {
				methods[idx] = m
			}
		}
		if !changed {
			return orig
		}
		return typ.NewInterface(v.Name, methods)
	case *typ.Ref, *typ.Generic:
		return orig
	default:
		return orig
	}
}

func expandInstantiatedCanDescend(t typ.Type) bool {
	switch t.Kind() {
	case kind.Optional,
		kind.Union,
		kind.Intersection,
		kind.Array,
		kind.Map,
		kind.Tuple,
		kind.Function,
		kind.Record,
		kind.Alias,
		kind.Interface,
		kind.Instantiated:
		return true
	default:
		return false
	}
}
