package synth

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// FieldResolver provides the minimal interface needed for field access resolution.
//
// Allows ResolveFieldAccess to be used with different synthesis implementations
// that can provide type lookup and field resolution. Both Engine and test mocks
// can implement this interface.
type FieldResolver interface {
	TypeOf(expr ast.Expr, p cfg.Point) typ.Type
	Field(t typ.Type, name string) (typ.Type, bool)
}

// ResolveFieldAccess resolves field or index access on an object type.
//
// This function handles the complexity of Lua's dynamic field access:
//
// For named field access (obj.field):
//   - Records: Returns the declared field type
//   - Interfaces: Returns the declared field type
//   - Unions: Checks all variants have the field
//   - Other types: Returns SkipCheck=true (dynamic access)
//
// For computed index access (obj[expr]):
//   - Maps: Returns the value type
//   - Arrays: Returns the element type
//   - Tuples: Returns the element type
//   - Other types: Returns NotIndexable=true if not indexable
//
// Special cases:
//   - Placeholder types (unknown, pending): Always SkipCheck=true
//   - IdentExpr keys: Treated as dynamic lookup, SkipCheck=true
//   - Empty fieldName with computed key: Checks indexability only
//
// If fullExpr is provided and resolver can type it directly, uses that
// type (for cached/known expressions). Otherwise, performs structural lookup.
func ResolveFieldAccess(
	resolver FieldResolver,
	fullExpr *ast.AttrGetExpr,
	objType typ.Type,
	fieldName string,
	p cfg.Point,
) FieldAccessResult {
	if resolver != nil && fullExpr != nil {
		fullType := resolver.TypeOf(fullExpr, p)
		if fullType != nil && !fullType.Kind().IsPlaceholder() {
			return FieldAccessResult{Type: fullType, Found: true, SkipCheck: true}
		}
	}

	if objType == nil {
		return FieldAccessResult{SkipCheck: true}
	}

	if objType.Kind().IsPlaceholder() {
		return FieldAccessResult{SkipCheck: true}
	}

	unwrapped := unwrap.Alias(objType)

	if fullExpr != nil {
		if _, isIdent := fullExpr.Key.(*ast.IdentExpr); isIdent {
			return FieldAccessResult{SkipCheck: true}
		}
	}

	if fieldName == "" {
		switch unwrapped.(type) {
		case *typ.Map, *typ.Array, *typ.Tuple, *typ.Record:
			return FieldAccessResult{SkipCheck: true}
		case *typ.TypeParam, *typ.Instantiated:
			return FieldAccessResult{SkipCheck: true}
		default:
			return FieldAccessResult{NotIndexable: true}
		}
	}

	switch unwrapped := unwrapped.(type) {
	case *typ.Record, *typ.Interface, *typ.Union, *typ.Intersection, *typ.Optional:
	case *typ.Map, *typ.Array, *typ.Tuple:
	case *typ.TypeParam, *typ.Instantiated:
		return FieldAccessResult{SkipCheck: true}
	case *typ.Function:
		return FieldAccessResult{NotIndexable: true}
	case *typ.Literal:
		if unwrapped.Base == kind.String {
			break
		}
		return FieldAccessResult{NotIndexable: true}
	default:
		if unwrapped == typ.Nil {
			return FieldAccessResult{NotIndexable: true}
		}
		if unwrapped.Kind() == kind.String {
			break
		}
		return FieldAccessResult{NotIndexable: true}
	}

	if resolver != nil {
		if ft, ok := resolver.Field(objType, fieldName); ok {
			return FieldAccessResult{Type: ft, Found: true}
		}
	}

	return FieldAccessResult{Found: false}
}
