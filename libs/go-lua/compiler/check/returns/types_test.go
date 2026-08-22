package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestLocalFuncInfoStructure(t *testing.T) {
	t.Run("struct fields are accessible", func(t *testing.T) {
		info := LocalFuncInfo{
			Sym:        cfg.SymbolID(1),
			Fn:         &ast.FunctionExpr{},
			DefScope:   scope.New(),
			Graph:      &cfg.Graph{},
			ParentFn:   nil,
			DefPoint:   cfg.Point(0),
			ParamHints: []typ.Type{typ.String, typ.Number},
		}

		if info.Sym != cfg.SymbolID(1) {
			t.Fatalf("expected Sym=1, got %v", info.Sym)
		}
		if info.Fn == nil {
			t.Fatal("expected non-nil Fn")
		}
		if info.DefScope == nil {
			t.Fatal("expected non-nil DefScope")
		}
		if info.Graph == nil {
			t.Fatal("expected non-nil Graph")
		}
		if info.ParentFn != nil {
			t.Fatal("expected nil ParentFn")
		}
		if info.DefPoint != cfg.Point(0) {
			t.Fatalf("expected DefPoint=0, got %v", info.DefPoint)
		}
		if len(info.ParamHints) != 2 {
			t.Fatalf("expected 2 ParamHints, got %d", len(info.ParamHints))
		}
	})

	t.Run("zero value is valid", func(t *testing.T) {
		var info LocalFuncInfo
		if info.Sym != 0 {
			t.Fatalf("expected zero Sym, got %v", info.Sym)
		}
		if info.Fn != nil {
			t.Fatal("expected nil Fn")
		}
		if info.ParamHints != nil {
			t.Fatal("expected nil ParamHints")
		}
	})
}

func TestMaxReturnSummaryIterations(t *testing.T) {
	t.Run("constant value", func(t *testing.T) {
		if MaxReturnSummaryIterations != 10 {
			t.Fatalf("expected MaxReturnSummaryIterations=10, got %d", MaxReturnSummaryIterations)
		}
	})

	t.Run("constant is positive", func(t *testing.T) {
		if MaxReturnSummaryIterations <= 0 {
			t.Fatal("expected positive constant")
		}
	})
}
