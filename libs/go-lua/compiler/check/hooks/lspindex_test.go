package hooks

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/query/core"
)

func TestLSPIndexer_EmptyFunction(t *testing.T) {
	symbols := index.NewSymbolIndex()
	indexer := NewLSPIndexer(symbols, nil)

	// Test with nil result
	indexer.extractFromFunction(nil, nil, nil)

	// Test with empty result
	sess := &check.Session{SourceName: "test.lua"}
	result := &api.FuncResult{}
	indexer.extractFromFunction(sess, nil, result)

	// Should not panic and should handle gracefully
	if len(symbols.SymbolsInFile("test.lua")) != 0 {
		t.Error("Expected no symbols for empty function")
	}
}

func TestAstSpan_NilNode(t *testing.T) {
	span := astSpan(nil)
	if span.Valid() {
		t.Error("Expected invalid span for nil node")
	}
}

func TestLSPIndexer_NestedCallEdges(t *testing.T) {
	callGraph := indexCalls(t, `
		local function bar(x)
			return x
		end

		local function foo(x)
			return x
		end

		local function outer()
			local y = foo(bar(1))
		end
	`)

	if !hasCallEdge(callGraph, "foo", 10) {
		t.Fatal("expected call edge for foo at line 10")
	}
	if !hasCallEdge(callGraph, "bar", 10) {
		t.Fatal("expected call edge for bar at line 10")
	}
}

func TestLSPIndexer_MemberCallEdges(t *testing.T) {
	callGraph := indexCalls(t, `
		local obj = {}
		function obj.foo(x) return x end
		function obj:bar(x) return x end

		local function outer()
			obj.foo(1)
			obj:bar(2)
		end
	`)

	if !hasCallEdge(callGraph, "foo", 6) {
		t.Fatal("expected call edge for obj.foo at line 6")
	}
	if !hasCallEdge(callGraph, "bar", 7) {
		t.Fatal("expected call edge for obj:bar at line 7")
	}
}

func hasCallEdge(callGraph *index.CallGraph, name string, line int) bool {
	if callGraph == nil {
		return false
	}
	for _, edge := range callGraph.AllEdges() {
		if edge.CalleeName == name && edge.CallSpan.StartLine == line {
			return true
		}
	}
	return false
}

func indexCalls(t *testing.T, source string) *index.CallGraph {
	t.Helper()

	source = strings.TrimSpace(source)

	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()
	indexer := NewLSPIndexer(symbols, callGraph)

	engine := core.NewEngine()
	checker := check.NewChecker(db.New(), check.Deps{
		Types:       engine,
		Stdlib:      scope.New(),
		GlobalTypes: nil,
		Resolver: &core.FuncResolver{
			FieldFunc: core.Field,
			IndexFunc: core.Index,
		},
	}, WithLSPIndex(indexer))

	checker.Check(source, "test.lua")

	return callGraph
}
