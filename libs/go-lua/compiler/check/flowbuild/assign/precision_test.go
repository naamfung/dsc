package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPreferPreciseDirectSourceType_PrefersNamedEquivalentAliasForSingleTarget(t *testing.T) {
	record := typ.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Integer).
		Build()
	alias := typ.NewAlias("Counter", record)

	got := preferPreciseDirectSourceType(
		record,
		&ast.IdentExpr{Value: "x"},
		0,
		nil,
		func(ast.Expr, cfg.Point) typ.Type { return alias },
		true,
	)
	if got != alias {
		t.Fatalf("expected direct named alias to win, got %s", typ.FormatShort(got))
	}
}

func TestPreferPreciseDirectSourceType_DoesNotReplaceNamedAssignedType(t *testing.T) {
	record := typ.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Integer).
		Build()
	alias := typ.NewAlias("Counter", record)

	got := preferPreciseDirectSourceType(
		alias,
		&ast.IdentExpr{Value: "x"},
		0,
		nil,
		func(ast.Expr, cfg.Point) typ.Type { return record },
		true,
	)
	if got != alias {
		t.Fatalf("expected existing named assigned type to remain, got %s", typ.FormatShort(got))
	}
}
