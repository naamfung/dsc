package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestExtractAssignTarget(t *testing.T) {
	t.Run("ident", func(t *testing.T) {
		expr := &ast.IdentExpr{Value: "x"}
		target := ExtractAssignTarget(expr)
		if target.Kind != TargetIdent {
			t.Error("Should be TargetIdent")
		}
		if target.Name != "x" {
			t.Errorf("Name should be 'x', got %q", target.Name)
		}
	})

	t.Run("field", func(t *testing.T) {
		expr := &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "obj"},
			Key:    &ast.StringExpr{Value: "prop"},
		}
		target := ExtractAssignTarget(expr)
		if target.Kind != TargetField {
			t.Error("Should be TargetField")
		}
		if target.BaseName != "obj" {
			t.Errorf("BaseName should be 'obj', got %q", target.BaseName)
		}
		if len(target.FieldPath) != 1 || target.FieldPath[0] != "prop" {
			t.Errorf("FieldPath should be [prop], got %v", target.FieldPath)
		}
	})

	t.Run("index", func(t *testing.T) {
		expr := &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "arr"},
			Key:    &ast.NumberExpr{Value: "1"},
		}
		target := ExtractAssignTarget(expr)
		if target.Kind != TargetIndex {
			t.Error("Should be TargetIndex for numeric key")
		}
	})

	t.Run("complex", func(t *testing.T) {
		expr := &ast.AttrGetExpr{
			Object: &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}},
			Key:    &ast.StringExpr{Value: "x"},
		}
		target := ExtractAssignTarget(expr)
		if target.Kind != TargetIndex {
			t.Error("Should be TargetIndex for non-path base")
		}
	})
}

func TestBuildCallInfo(t *testing.T) {
	t.Run("simple call", func(t *testing.T) {
		call := &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "print"},
			Args: []ast.Expr{
				&ast.IdentExpr{Value: "a"},
				&ast.NumberExpr{Value: "1"},
			},
		}

		info := BuildCallInfo(call, true)

		if info.CalleeName != "print" {
			t.Errorf("CalleeName = %q, want 'print'", info.CalleeName)
		}
		if !info.IsStmt {
			t.Error("IsStmt should be true")
		}
		if len(info.ArgNames) != 2 {
			t.Errorf("Should have 2 arg names, got %d", len(info.ArgNames))
		}
		if info.ArgNames[0] != "a" {
			t.Errorf("ArgNames[0] = %q, want 'a'", info.ArgNames[0])
		}
	})

	t.Run("method call", func(t *testing.T) {
		call := &ast.FuncCallExpr{
			Receiver: &ast.IdentExpr{Value: "obj"},
			Method:   "doIt",
			Args:     []ast.Expr{},
		}

		info := BuildCallInfo(call, false)

		if info.Method != "doIt" {
			t.Errorf("Method = %q, want 'doIt'", info.Method)
		}
		if info.ReceiverName != "obj" {
			t.Errorf("ReceiverName = %q, want 'obj'", info.ReceiverName)
		}
	})

	t.Run("nil call", func(t *testing.T) {
		info := BuildCallInfo(nil, false)
		if info != nil {
			t.Error("Nil call should return nil")
		}
	})
}

func TestExtractTypeCheckPattern(t *testing.T) {
	t.Run("Type:is pattern", func(t *testing.T) {
		info := &CallInfo{
			Method:       "is",
			ReceiverName: "String",
			Args:         []ast.Expr{&ast.IdentExpr{Value: "val"}},
		}
		ExtractTypeCheckPattern(info)

		if !info.IsTypeCheck {
			t.Error("Should be detected as type check")
		}
		if info.TypeCheckName != "String" {
			t.Errorf("TypeCheckName = %q, want 'String'", info.TypeCheckName)
		}
	})

	t.Run("TypeName(x) pattern", func(t *testing.T) {
		info := &CallInfo{
			CalleeName: "Number",
			Args:       []ast.Expr{&ast.IdentExpr{Value: "x"}},
		}
		ExtractTypeCheckPattern(info)

		if !info.IsTypeCheck {
			t.Error("Should be detected as type check")
		}
		if info.TypeCheckName != "Number" {
			t.Errorf("TypeCheckName = %q, want 'Number'", info.TypeCheckName)
		}
	})

	t.Run("nil info", func(_ *testing.T) {
		ExtractTypeCheckPattern(nil)
	})
}

func TestExtractSourceCalls(t *testing.T) {
	exprs := []ast.Expr{
		&ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "foo"},
			Args: []ast.Expr{},
		},
		&ast.NumberExpr{Value: "42"},
		&ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "bar"},
			Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
		},
	}

	calls := ExtractSourceCalls(exprs)

	if len(calls) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(calls))
	}
	if calls[0] == nil || calls[0].CalleeName != "foo" {
		t.Error("calls[0] should be foo")
	}
	if calls[1] != nil {
		t.Error("calls[1] should be nil for non-call")
	}
	if calls[2] == nil || calls[2].CalleeName != "bar" {
		t.Error("calls[2] should be bar")
	}
}

func TestExtractSourceCalls_Empty(t *testing.T) {
	calls := ExtractSourceCalls(nil)
	if calls != nil {
		t.Error("Nil exprs should return nil")
	}

	calls = ExtractSourceCalls([]ast.Expr{})
	if calls != nil {
		t.Error("Empty exprs should return nil")
	}
}
