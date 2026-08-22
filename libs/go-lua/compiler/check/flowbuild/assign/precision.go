package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// preferPreciseDirectSourceType keeps assignment inference on the canonical
// expression-synthesis path.
//
// Direct synthesis is allowed to repair a slot only when it is strictly more
// informative than the expanded assignment value. This keeps tuple expansion as
// the primary source of truth for assignment slots while still allowing
// canonical single-expression synthesis to repair top-like degradation.
func preferPreciseDirectSourceType(
	assignedType typ.Type,
	source ast.Expr,
	p cfg.Point,
	sc *scope.State,
	synth func(ast.Expr, cfg.Point) typ.Type,
	singleTarget bool,
) typ.Type {
	if source == nil || synth == nil {
		return assignedType
	}
	switch source.(type) {
	case *ast.Comma3Expr:
		return assignedType
	}

	precise := resolve.Ref(synth(source, p), sc)
	if typ.IsAbsentOrUnknown(precise) {
		return assignedType
	}
	if singleTarget {
		if typ.IsAbsentOrUnknown(assignedType) || typ.IsAny(assignedType) {
			return precise
		}
		if subtype.IsSubtype(precise, assignedType) && !subtype.IsSubtype(assignedType, precise) {
			return precise
		}
		if preferNamedEquivalentDirectType(precise, assignedType) {
			return precise
		}
		return assignedType
	}
	if typ.IsAny(assignedType) && !typ.IsAny(precise) {
		return precise
	}
	return assignedType
}

func preferNamedEquivalentDirectType(precise, assignedType typ.Type) bool {
	if !isNamedIdentityType(precise) || isNamedIdentityType(assignedType) {
		return false
	}
	return subtype.IsSubtype(precise, assignedType) && subtype.IsSubtype(assignedType, precise)
}

func isNamedIdentityType(t typ.Type) bool {
	switch t.(type) {
	case *typ.Alias, *typ.Ref:
		return true
	default:
		return false
	}
}
