package ops

import (
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// ExpectedTableElementType returns the contextual expected type for the element
// at index in a table literal used in array-like position.
func ExpectedTableElementType(expected typ.Type, index int) typ.Type {
	return expectedTableElementType(expected, index)
}

func expectedTableElementType(expected typ.Type, index int) typ.Type {
	if expected == nil {
		return nil
	}

	expected = typ.UnwrapAnnotated(expected)

	switch v := expected.(type) {
	case *typ.Alias:
		return expectedTableElementType(v.Target, index)
	case *typ.Optional:
		return expectedTableElementType(v.Inner, index)
	case *typ.Instantiated:
		if resolved, err := querycore.ResolveInstantiated(v); err == nil {
			return expectedTableElementType(resolved, index)
		}
		return nil
	case *typ.Array:
		return v.Element
	case *typ.Tuple:
		if index < 0 || index >= len(v.Elements) {
			return nil
		}
		return v.Elements[index]
	case *typ.Map:
		if v.Key == nil {
			return nil
		}
		switch v.Key.Kind() {
		case kind.Integer, kind.Number:
			return v.Value
		default:
			return nil
		}
	case *typ.Record:
		if !v.HasMapComponent() || v.MapKey == nil {
			return nil
		}
		switch v.MapKey.Kind() {
		case kind.Integer, kind.Number:
			return v.MapValue
		default:
			return nil
		}
	case *typ.Union:
		var members []typ.Type
		for _, member := range v.Members {
			if elem := expectedTableElementType(member, index); elem != nil {
				members = append(members, elem)
			}
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewUnion(members...)
	case *typ.Intersection:
		var members []typ.Type
		for _, member := range v.Members {
			if elem := expectedTableElementType(member, index); elem != nil {
				members = append(members, elem)
			}
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewIntersection(members...)
	default:
		return nil
	}
}
