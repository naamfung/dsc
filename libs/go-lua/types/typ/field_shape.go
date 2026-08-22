package typ

import "github.com/wippyai/go-lua/types/kind"

// SplitNilableFieldType converts a nil-capable field value type into the shape
// used by table and record fields. In Lua, assigning nil to a table field
// removes the key, so a value expression of type T|nil in a table literal or
// field merge is represented as an optional field of type T rather than a
// required field of type T|nil.
func SplitNilableFieldType(t Type) (inner Type, optional bool) {
	if t == nil {
		return Unknown, true
	}
	if a, ok := t.(*Alias); ok {
		if a == nil || a.Target == nil {
			return t, false
		}
		if opt, ok := a.Target.(*Optional); ok && opt != nil && opt.Inner != nil {
			return opt.Inner, true
		}
		if u, ok := a.Target.(*Union); ok && u != nil && len(u.Members) > 0 {
			return splitNilableUnionMembers(u.Members, t)
		}
		return t, false
	}
	if opt, ok := t.(*Optional); ok && opt != nil && opt.Inner != nil {
		return opt.Inner, true
	}
	if u, ok := t.(*Union); ok && u != nil && len(u.Members) > 0 {
		return splitNilableUnionMembers(u.Members, t)
	}
	return t, false
}

func splitNilableUnionMembers(members []Type, original Type) (Type, bool) {
	hasNil := false
	nonNil := make([]Type, 0, len(members))
	for _, m := range members {
		if m != nil && m.Kind() == kind.Nil {
			hasNil = true
			continue
		}
		nonNil = append(nonNil, m)
	}
	if !hasNil {
		return original, false
	}
	switch len(nonNil) {
	case 0:
		return Nil, true
	case 1:
		return nonNil[0], true
	default:
		return NewUnion(nonNil...), true
	}
}
