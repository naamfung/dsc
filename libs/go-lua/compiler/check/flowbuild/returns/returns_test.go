package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/types/flow"
)

func TestFlowContext_ZeroValue(t *testing.T) {
	var fc core.FlowContext
	if fc.Graph != nil {
		t.Error("expected nil Graph in zero value")
	}
	if fc.Scopes != nil {
		t.Error("expected nil Scopes in zero value")
	}
	if fc.Derived != nil {
		t.Error("expected nil Derived in zero value")
	}
}

func TestClassifyReturnExpr_TrueLiteral(t *testing.T) {
	expr := &ast.TrueExpr{}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnTrue {
		t.Errorf("expected ReturnTrue for TrueExpr, got %v", kind)
	}
}

func TestClassifyReturnExpr_FalseLiteral(t *testing.T) {
	expr := &ast.FalseExpr{}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnFalse {
		t.Errorf("expected ReturnFalse for FalseExpr, got %v", kind)
	}
}

func TestClassifyReturnExpr_NilLiteral(t *testing.T) {
	expr := &ast.NilExpr{}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnUnknown {
		t.Errorf("expected ReturnUnknown for NilExpr, got %v", kind)
	}
}

func TestClassifyReturnExpr_NumberExpr(t *testing.T) {
	expr := &ast.NumberExpr{Value: "42"}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnUnknown {
		t.Errorf("expected ReturnUnknown for NumberExpr, got %v", kind)
	}
}

func TestClassifyReturnExpr_StringExpr(t *testing.T) {
	expr := &ast.StringExpr{Value: "hello"}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnUnknown {
		t.Errorf("expected ReturnUnknown for StringExpr, got %v", kind)
	}
}

func TestClassifyReturnExpr_IdentExpr(t *testing.T) {
	expr := &ast.IdentExpr{Value: "someVar"}
	kind := resolve.ClassifyReturnExpr(expr)
	if kind != flow.ReturnUnknown {
		t.Errorf("expected ReturnUnknown for IdentExpr, got %v", kind)
	}
}

func TestClassifyReturnExpr_NilInput(t *testing.T) {
	kind := resolve.ClassifyReturnExpr(nil)
	if kind != flow.ReturnUnknown {
		t.Errorf("expected ReturnUnknown for nil, got %v", kind)
	}
}
