package transform

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExprMatchesLiteral_IntegerSyntaxVariants(t *testing.T) {
	tests := []struct {
		expr string
		want int64
	}{
		{expr: "08", want: 8},
		{expr: "0xDEAD", want: 0xDEAD},
	}

	for _, tt := range tests {
		ok := exprMatchesLiteral(&ast.NumberExpr{Value: tt.expr}, typ.LiteralInt(tt.want))
		if !ok {
			t.Fatalf("exprMatchesLiteral(%q, %d) = false, want true", tt.expr, tt.want)
		}
	}
}

func TestExprMatchesLiteral_FloatLiteral(t *testing.T) {
	if !exprMatchesLiteral(&ast.NumberExpr{Value: "0x1p2"}, typ.LiteralNumber(4)) {
		t.Fatal("expected hex float literal to match literal number 4")
	}
}

func TestExprMatchesLiteral_HexFloatDoesNotMatchInteger(t *testing.T) {
	if exprMatchesLiteral(&ast.NumberExpr{Value: "0x1p2"}, typ.LiteralInt(4)) {
		t.Fatal("expected hex float literal to not match integer literal 4")
	}
}
