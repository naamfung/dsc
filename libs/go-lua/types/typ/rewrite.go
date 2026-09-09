package typ

import (
	"sync"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// Rewrite traverses a type tree and applies fn at each node (bottom-up transformation).
//
// The function fn is called on each type node before recursing into children.
// If fn returns (replacement, true), the replacement is used and children are
// not visited (early termination). If fn returns (_, false), children are
// recursively rewritten first, then the result is reassembled.
//
// Returns the original pointer when nothing changed (structural sharing).
// This is the foundation for type substitution, expansion, and other transforms.
func Rewrite(t Type, fn func(Type) (Type, bool)) Type {
	return rewriteWithDepth(t, fn, DefaultRecursionDepth)
}

func rewriteWithDepth(t Type, fn func(Type) (Type, bool), maxDepth int) Type {
	guard := GuardForDepth(maxDepth)
	if !rewriteCanDescend(t) {
		return rewriteDepth(t, fn, guard, nil)
	}
	memo := getRewriteMemo()
	defer putRewriteMemo(memo)
	return rewriteDepth(t, fn, guard, memo)
}

const rewriteMemoMaxEntries = 4096

var rewriteMemoPool = sync.Pool{
	New: func() any {
		return make(map[rewriteKey]Type, 64)
	},
}

func getRewriteMemo() map[rewriteKey]Type {
	return rewriteMemoPool.Get().(map[rewriteKey]Type)
}

func putRewriteMemo(m map[rewriteKey]Type) {
	if len(m) > rewriteMemoMaxEntries {
		rewriteMemoPool.Put(make(map[rewriteKey]Type, 64))
		return
	}
	clear(m)
	rewriteMemoPool.Put(m)
}

type rewriteKey struct {
	t     Type
	depth int
}

func rewriteDepth(t Type, fn func(Type) (Type, bool), guard internal.RecursionGuard, memo map[rewriteKey]Type) Type {
	if t == nil {
		return t
	}
	if !rewriteCanDescend(t) {
		if replacement, ok := fn(t); ok {
			return replacement
		}
		return t
	}

	depth := guard.Depth()
	var key rewriteKey
	if memo != nil {
		key = rewriteKey{t: t, depth: depth}
		if cached, ok := memo[key]; ok {
			return cached
		}
	}

	next, ok := guard.Enter(t)
	if !ok {
		return t
	}

	if replacement, ok := fn(t); ok {
		if memo != nil {
			memo[key] = replacement
		}
		return replacement
	}

	var out Type
	switch tt := unwrapTransparentWrappers(t).(type) {
	case *Optional:
		if tt.Inner == nil {
			out = t
			break
		}
		inner := rewriteDepth(tt.Inner, fn, next, memo)
		if inner == tt.Inner {
			out = t
			break
		}
		out = NewOptional(inner)
	case *Union:
		var members []Type
		for i, m := range tt.Members {
			newMember := rewriteDepth(m, fn, next, memo)
			if newMember != m {
				if members == nil {
					members = make([]Type, len(tt.Members))
					copy(members, tt.Members)
				}
				members[i] = newMember
			} else if members != nil {
				members[i] = m
			}
		}
		if members == nil {
			out = t
			break
		}
		out = NewUnion(members...)
	case *Intersection:
		var members []Type
		for i, m := range tt.Members {
			newMember := rewriteDepth(m, fn, next, memo)
			if newMember != m {
				if members == nil {
					members = make([]Type, len(tt.Members))
					copy(members, tt.Members)
				}
				members[i] = newMember
			} else if members != nil {
				members[i] = m
			}
		}
		if members == nil {
			out = t
			break
		}
		out = NewIntersection(members...)
	case *Array:
		elem := rewriteDepth(tt.Element, fn, next, memo)
		if elem == tt.Element {
			out = t
			break
		}
		out = NewArray(elem)
	case *Map:
		keyType := rewriteDepth(tt.Key, fn, next, memo)
		valueType := rewriteDepth(tt.Value, fn, next, memo)
		if keyType == tt.Key && valueType == tt.Value {
			out = t
			break
		}
		out = NewMap(keyType, valueType)
	case *Tuple:
		var elems []Type
		for i, e := range tt.Elements {
			newElem := rewriteDepth(e, fn, next, memo)
			if newElem != e {
				if elems == nil {
					elems = make([]Type, len(tt.Elements))
					copy(elems, tt.Elements)
				}
				elems[i] = newElem
			} else if elems != nil {
				elems[i] = e
			}
		}
		if elems == nil {
			out = t
			break
		}
		out = NewTuple(elems...)
	case *Function:
		out = rewriteFunction(tt, t, fn, next, memo)
	case *Record:
		out = rewriteRecord(tt, t, fn, next, memo)
	case *Alias:
		target := rewriteDepth(tt.Target, fn, next, memo)
		if target == tt.Target {
			out = t
			break
		}
		out = NewAlias(tt.Name, target)
	case *Instantiated:
		var args []Type
		for idx, a := range tt.TypeArgs {
			newArg := rewriteDepth(a, fn, next, memo)
			if newArg != a {
				if args == nil {
					args = make([]Type, len(tt.TypeArgs))
					copy(args, tt.TypeArgs)
				}
				args[idx] = newArg
			} else if args != nil {
				args[idx] = a
			}
		}
		if args == nil {
			out = t
			break
		}
		out = Instantiate(tt.Generic, args...)
	case *Interface:
		var methods []Method
		for idx, m := range tt.Methods {
			newType := rewriteDepth(m.Type, fn, next, memo)
			if newType != m.Type {
				if methods == nil {
					methods = make([]Method, len(tt.Methods))
					copy(methods, tt.Methods)
				}
				if fnType, ok := newType.(*Function); ok {
					methods[idx] = Method{Name: m.Name, Type: fnType}
				} else {
					methods[idx] = m
				}
			} else if methods != nil {
				methods[idx] = m
			}
		}
		if methods == nil {
			out = t
			break
		}
		out = NewInterface(tt.Name, methods)
	default:
		out = t
	}

	if memo != nil {
		memo[key] = out
	}
	return out
}

func rewriteCanDescend(t Type) bool {
	if t == nil {
		return false
	}
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
		kind.Instantiated,
		kind.Interface:
		return true
	default:
		return false
	}
}

func rewriteFunction(v *Function, orig Type, fn func(Type) (Type, bool), guard internal.RecursionGuard, memo map[rewriteKey]Type) Type {
	changed := false

	var params []Param
	for i, p := range v.Params {
		newType := rewriteDepth(p.Type, fn, guard, memo)
		if newType != p.Type {
			if params == nil {
				params = make([]Param, len(v.Params))
				copy(params, v.Params)
			}
			changed = true
			params[i] = Param{Name: p.Name, Type: newType, Optional: p.Optional}
		} else if params != nil {
			params[i] = p
		}
	}

	var returns []Type
	for i, r := range v.Returns {
		newRet := rewriteDepth(r, fn, guard, memo)
		if newRet != r {
			if returns == nil {
				returns = make([]Type, len(v.Returns))
				copy(returns, v.Returns)
			}
			changed = true
			returns[i] = newRet
		} else if returns != nil {
			returns[i] = r
		}
	}

	var variadic Type
	if v.Variadic != nil {
		variadic = rewriteDepth(v.Variadic, fn, guard, memo)
		if variadic != v.Variadic {
			changed = true
		}
	}

	if !changed {
		return orig
	}

	paramSrc := v.Params
	if params != nil {
		paramSrc = params
	}
	returnsSrc := v.Returns
	if returns != nil {
		returnsSrc = returns
	}
	return buildFunctionType(
		v.TypeParams,
		paramSrc,
		variadic,
		returnsSrc,
		v.Effects,
		v.Spec,
		v.Refinement,
	)
}

func rewriteRecord(v *Record, orig Type, fn func(Type) (Type, bool), guard internal.RecursionGuard, memo map[rewriteKey]Type) Type {
	changed := false

	var fields []Field
	for i, f := range v.Fields {
		newType := rewriteDepth(f.Type, fn, guard, memo)
		if newType != f.Type {
			if fields == nil {
				fields = make([]Field, len(v.Fields))
				copy(fields, v.Fields)
			}
			changed = true
			fields[i] = Field{Name: f.Name, Type: newType, Optional: f.Optional, Readonly: f.Readonly}
		} else if fields != nil {
			fields[i] = f
		}
	}

	var metatable Type
	if v.Metatable != nil {
		metatable = rewriteDepth(v.Metatable, fn, guard, memo)
		if metatable != v.Metatable {
			changed = true
		}
	}

	mapKey := v.MapKey
	mapValue := v.MapValue
	if v.HasMapComponent() {
		mapKey = rewriteDepth(v.MapKey, fn, guard, memo)
		if mapKey != v.MapKey {
			changed = true
		}
		mapValue = rewriteDepth(v.MapValue, fn, guard, memo)
		if mapValue != v.MapValue {
			changed = true
		}
	}

	if !changed {
		return orig
	}

	fieldsSrc := v.Fields
	if fields != nil {
		fieldsSrc = fields
	}
	return buildRecordType(fieldsSrc, metatable, mapKey, mapValue, v.Open, true)
}
