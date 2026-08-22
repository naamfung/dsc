package modules

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestCollectAliases_NilGraph(t *testing.T) {
	aliases := CollectAliases(nil)
	if aliases != nil {
		t.Errorf("expected nil for nil graph, got %v", aliases)
	}
}

func TestCollectAliases_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)
	result := CollectAliases(graph)
	if result != nil {
		t.Error("expected nil for empty graph")
	}
}
