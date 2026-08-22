package typ

import (
	"sync"

	"github.com/wippyai/go-lua/internal"
)

// SoftPolicy controls how soft-placeholder detection behaves.
type SoftPolicy struct {
	// AllowEmptyRecord treats {} as a soft placeholder when true.
	AllowEmptyRecord bool
}

// SoftAnnotationPolicy treats empty records as non-soft (annotation semantics).
var SoftAnnotationPolicy = SoftPolicy{}

// SoftPlaceholderPolicy treats empty records as soft (placeholder semantics).
var SoftPlaceholderPolicy = SoftPolicy{AllowEmptyRecord: true}

// IsSoft reports whether a type should be treated as a soft placeholder under policy.
func IsSoft(t Type, policy SoftPolicy) bool {
	return isSoft(t, NewGuard(), policy)
}

func isSoft(t Type, guard internal.RecursionGuard, policy SoftPolicy) bool {
	if t == nil {
		return false
	}
	node := unwrapTransparentSoft(t)
	switch node.(type) {
	case *Alias, *Optional, *Array, *Map, *Record, *Union:
		// recurse below
	default:
		return node.Kind().IsPlaceholder()
	}

	next, ok := guard.Enter(node)
	if !ok {
		return false
	}

	switch tt := node.(type) {
	case *Alias:
		return isSoft(tt.Target, next, policy)
	case *Optional:
		return isSoft(tt.Inner, next, policy)
	case *Array:
		return isSoft(tt.Element, next, policy)
	case *Map:
		return isSoft(tt.Value, next, policy)
	case *Record:
		if len(tt.Fields) == 0 && !tt.HasMapComponent() {
			return policy.AllowEmptyRecord
		}
		if tt.HasMapComponent() && len(tt.Fields) == 0 {
			return isSoft(tt.MapValue, next, policy)
		}
		return false
	case *Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, m := range tt.Members {
			if !isSoft(m, next, policy) {
				return false
			}
		}
		return true
	}
	return false
}

func unwrapTransparentSoft(t Type) Type {
	for {
		ann, ok := t.(*Annotated)
		if !ok {
			return t
		}
		if ann.Inner == nil || ann.Inner == t {
			return t
		}
		t = ann.Inner
	}
}

// PruneSoftUnionMembers removes soft placeholder members from unions when
// any non-soft member is present. This helps inference avoid contaminating
// concrete types with placeholder unions.
func PruneSoftUnionMembers(t Type) Type {
	if t == nil {
		return nil
	}
	if !softPruneMayRewrite(t) {
		return t
	}
	state := getSoftPruneState()
	defer putSoftPruneState(state)
	guard := NewGuard()
	return pruneSoftUnionMembersMemo(t, guard, state.memo, state.visiting, state.softMemo)
}

const softPruneMemoMaxEntries = 4096

type softPruneState struct {
	memo     map[Type]Type
	visiting map[Type]struct{}
	softMemo map[Type]bool
}

var softPruneStatePool = sync.Pool{
	New: func() any {
		return &softPruneState{
			memo:     make(map[Type]Type, 64),
			visiting: make(map[Type]struct{}, 64),
			softMemo: make(map[Type]bool, 64),
		}
	},
}

func getSoftPruneState() *softPruneState {
	return softPruneStatePool.Get().(*softPruneState)
}

func putSoftPruneState(state *softPruneState) {
	if state == nil {
		return
	}
	if len(state.memo) > softPruneMemoMaxEntries {
		state.memo = make(map[Type]Type, 64)
	} else {
		clear(state.memo)
	}
	if len(state.visiting) > softPruneMemoMaxEntries {
		state.visiting = make(map[Type]struct{}, 64)
	} else {
		clear(state.visiting)
	}
	if len(state.softMemo) > softPruneMemoMaxEntries {
		state.softMemo = make(map[Type]bool, 64)
	} else {
		clear(state.softMemo)
	}
	softPruneStatePool.Put(state)
}

func pruneSoftUnionMembersMemo(
	t Type,
	guard internal.RecursionGuard,
	memo map[Type]Type,
	visiting map[Type]struct{},
	softMemo map[Type]bool,
) Type {
	if t == nil {
		return t
	}
	if !softPruneMayRewrite(t) {
		return t
	}
	if cached, ok := memo[t]; ok {
		return cached
	}
	if _, ok := visiting[t]; ok {
		return t
	}
	next, ok := guard.Enter(t)
	if !ok {
		return t
	}
	visiting[t] = struct{}{}

	var out Type
	switch node := unwrapTransparentSoft(t).(type) {
	case *Function:
		out = pruneSoftFunction(node, t, next, memo, visiting, softMemo)
	case *Record:
		out = pruneSoftRecord(node, t, next, memo, visiting, softMemo)
	case *Union:
		// Fast path: if this union has no soft members and no nested changes needed,
		// skip the expensive isSoftWithMemo checks entirely.
		if !node.HasSoftMember() {
			anyChildChanged := false
			var rewrittenFast []Type
			for idx, m := range node.Members {
				pm := pruneSoftUnionMembersMemo(m, next, memo, visiting, softMemo)
				if pm != m {
					if rewrittenFast == nil {
						rewrittenFast = make([]Type, len(node.Members))
						copy(rewrittenFast, node.Members)
					}
					rewrittenFast[idx] = pm
					anyChildChanged = true
				} else if rewrittenFast != nil {
					rewrittenFast[idx] = m
				}
			}
			if !anyChildChanged {
				out = t
			} else {
				out = NewUnion(rewrittenFast...)
			}
			break
		}

		var rewritten []Type
		softCount := 0
		changed := false
		for idx, m := range node.Members {
			pm := pruneSoftUnionMembersMemo(m, next, memo, visiting, softMemo)
			if pm != m {
				if rewritten == nil {
					rewritten = make([]Type, len(node.Members))
					copy(rewritten, node.Members)
				}
				rewritten[idx] = pm
				changed = true
			} else if rewritten != nil {
				rewritten[idx] = m
			}
			if isSoftWithMemo(pm, SoftPlaceholderPolicy, softMemo) {
				softCount++
			}
		}
		if softCount > 0 && softCount < len(node.Members) {
			members := node.Members
			if rewritten != nil {
				members = rewritten
			}
			nonSoftMembers := make([]Type, 0, len(node.Members)-softCount)
			for _, member := range members {
				if !isSoftWithMemo(member, SoftPlaceholderPolicy, softMemo) {
					nonSoftMembers = append(nonSoftMembers, member)
				}
			}
			out = NewUnion(nonSoftMembers...)
			break
		}
		if !changed {
			out = t
			break
		}
		members := node.Members
		if rewritten != nil {
			members = rewritten
		}
		out = NewUnion(members...)
	case *Optional:
		if node.Inner == nil {
			out = t
			break
		}
		inner := pruneSoftUnionMembersMemo(node.Inner, next, memo, visiting, softMemo)
		if inner == node.Inner {
			out = t
			break
		}
		out = NewOptional(inner)
	case *Array:
		elem := pruneSoftUnionMembersMemo(node.Element, next, memo, visiting, softMemo)
		if elem == node.Element {
			out = t
			break
		}
		out = NewArray(elem)
	case *Map:
		key := pruneSoftUnionMembersMemo(node.Key, next, memo, visiting, softMemo)
		val := pruneSoftUnionMembersMemo(node.Value, next, memo, visiting, softMemo)
		if key == node.Key && val == node.Value {
			out = t
			break
		}
		out = NewMap(key, val)
	case *Tuple:
		var elems []Type
		for i, e := range node.Elements {
			newElem := pruneSoftUnionMembersMemo(e, next, memo, visiting, softMemo)
			if newElem != e {
				if elems == nil {
					elems = make([]Type, len(node.Elements))
					copy(elems, node.Elements)
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
	case *Alias:
		target := pruneSoftUnionMembersMemo(node.Target, next, memo, visiting, softMemo)
		if target == node.Target {
			out = t
			break
		}
		out = NewAlias(node.Name, target)
	case *Instantiated:
		var args []Type
		for idx, a := range node.TypeArgs {
			newArg := pruneSoftUnionMembersMemo(a, next, memo, visiting, softMemo)
			if newArg != a {
				if args == nil {
					args = make([]Type, len(node.TypeArgs))
					copy(args, node.TypeArgs)
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
		out = Instantiate(node.Generic, args...)
	default:
		out = t
	}

	delete(visiting, t)
	memo[t] = out
	return out
}

func pruneSoftFunction(
	f *Function,
	orig Type,
	next internal.RecursionGuard,
	memo map[Type]Type,
	visiting map[Type]struct{},
	softMemo map[Type]bool,
) Type {
	changed := false

	var params []Param
	for i, p := range f.Params {
		newType := pruneSoftUnionMembersMemo(p.Type, next, memo, visiting, softMemo)
		if newType != p.Type {
			if params == nil {
				params = make([]Param, len(f.Params))
				copy(params, f.Params)
			}
			changed = true
			params[i] = Param{Name: p.Name, Type: newType, Optional: p.Optional}
		} else if params != nil {
			params[i] = p
		}
	}

	var returns []Type
	for i, r := range f.Returns {
		newRet := pruneSoftUnionMembersMemo(r, next, memo, visiting, softMemo)
		if newRet != r {
			if returns == nil {
				returns = make([]Type, len(f.Returns))
				copy(returns, f.Returns)
			}
			changed = true
			returns[i] = newRet
		} else if returns != nil {
			returns[i] = r
		}
	}

	variadic := f.Variadic
	if f.Variadic != nil {
		newVariadic := pruneSoftUnionMembersMemo(f.Variadic, next, memo, visiting, softMemo)
		if newVariadic != f.Variadic {
			changed = true
			variadic = newVariadic
		}
	}

	if !changed {
		return orig
	}

	paramSrc := f.Params
	if params != nil {
		paramSrc = params
	}
	returnsSrc := f.Returns
	if returns != nil {
		returnsSrc = returns
	}
	return buildFunctionType(
		f.TypeParams,
		paramSrc,
		variadic,
		returnsSrc,
		f.Effects,
		f.Spec,
		f.Refinement,
	)
}

func pruneSoftRecord(
	r *Record,
	orig Type,
	next internal.RecursionGuard,
	memo map[Type]Type,
	visiting map[Type]struct{},
	softMemo map[Type]bool,
) Type {
	changed := false

	var fields []Field
	for i, f := range r.Fields {
		newType := pruneSoftUnionMembersMemo(f.Type, next, memo, visiting, softMemo)
		if newType != f.Type {
			if fields == nil {
				fields = make([]Field, len(r.Fields))
				copy(fields, r.Fields)
			}
			changed = true
			fields[i] = Field{Name: f.Name, Type: newType, Optional: f.Optional, Readonly: f.Readonly}
		} else if fields != nil {
			fields[i] = f
		}
	}

	metatable := r.Metatable
	if r.Metatable != nil {
		newMetatable := pruneSoftUnionMembersMemo(r.Metatable, next, memo, visiting, softMemo)
		if newMetatable != r.Metatable {
			changed = true
			metatable = newMetatable
		}
	}

	mapKey := r.MapKey
	mapValue := r.MapValue
	if r.HasMapComponent() {
		newMapKey := pruneSoftUnionMembersMemo(r.MapKey, next, memo, visiting, softMemo)
		if newMapKey != r.MapKey {
			changed = true
			mapKey = newMapKey
		}
		newMapValue := pruneSoftUnionMembersMemo(r.MapValue, next, memo, visiting, softMemo)
		if newMapValue != r.MapValue {
			changed = true
			mapValue = newMapValue
		}
	}

	if !changed {
		return orig
	}

	fieldsSrc := r.Fields
	if fields != nil {
		fieldsSrc = fields
	}
	return buildRecordType(fieldsSrc, metatable, mapKey, mapValue, r.Open, true)
}

func isSoftWithMemo(t Type, policy SoftPolicy, memo map[Type]bool) bool {
	if t == nil {
		return false
	}
	if cached, ok := memo[t]; ok {
		return cached
	}
	node := unwrapTransparentSoft(t)
	if node != t {
		if cached, ok := memo[node]; ok {
			memo[t] = cached
			return cached
		}
	}
	soft := IsSoft(node, policy)
	memo[t] = soft
	if node != t {
		memo[node] = soft
	}
	return soft
}
