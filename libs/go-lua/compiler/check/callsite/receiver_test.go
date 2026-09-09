package callsite

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	ccfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
	typecfg "github.com/wippyai/go-lua/types/cfg"
)

func TestForceMethodReceiver_DotDefinedFieldFunction(t *testing.T) {
	src := `
		local T = {}
		function T.foo(x: number): number
			return x + 1
		end
		local n = T:foo(1)
	`
	graph, bindings, call, point := parseGraphAndMethodCall(t, src)

	if !ForceMethodReceiver(bindings, graph, call) {
		t.Fatal("expected ForceMethodReceiver to be true for dot-defined field function")
	}
	if !ForceMethodReceiverAtPoint(bindings, graph, point, call.Call) {
		t.Fatal("expected ForceMethodReceiverAtPoint to be true for dot-defined field function")
	}
}

func TestForceMethodReceiver_FieldAssignedFunctionLiteral(t *testing.T) {
	src := `
		local T = {}
		T.foo = function(x: number): number
			return x + 1
		end
		local n = T:foo(1)
	`
	graph, bindings, call, point := parseGraphAndMethodCall(t, src)

	if !ForceMethodReceiver(bindings, graph, call) {
		t.Fatal("expected ForceMethodReceiver to be true for field-assigned function literal")
	}
	if !ForceMethodReceiverAtPoint(bindings, graph, point, call.Call) {
		t.Fatal("expected ForceMethodReceiverAtPoint to be true for field-assigned function literal")
	}
}

func TestForceMethodReceiver_UsesCalleePathWhenReceiverExprMissing(t *testing.T) {
	src := `
		local T = {}
		function T.foo(x: number): number
			return x + 1
		end
		local n = T:foo(1)
	`
	graph, bindings, call, _ := parseGraphAndMethodCall(t, src)
	callCopy := *call
	callCopy.Receiver = nil
	callCopy.CalleeSymbol = 0

	if !ForceMethodReceiver(bindings, graph, &callCopy) {
		t.Fatal("expected ForceMethodReceiver to resolve method symbol via CalleePath")
	}
}

func TestForceMethodReceiver_PrefersCanonicalCandidateOverStaleRawSymbol(t *testing.T) {
	src := `
		local T = {}
		function T.foo(x: number): number
			return x + 1
		end
		local stale = 1
		local n = T:foo(1)
	`
	graph, bindings, call, point := parseGraphAndMethodCall(t, src)
	staleSym, ok := graph.SymbolAt(point, "stale")
	if !ok || staleSym == 0 {
		t.Fatal("expected stale symbol in scope")
	}

	callCopy := *call
	callCopy.CalleeSymbol = staleSym

	if !ForceMethodReceiver(bindings, graph, &callCopy) {
		t.Fatal("expected ForceMethodReceiver to ignore stale raw symbol and use canonical method candidate")
	}
}

func TestForceMethodReceiver_UsesAliasReceiverBase(t *testing.T) {
	src := `
		local T = {}
		function T.foo(x: number): number
			return x + 1
		end
		local Alias = T
		local n = Alias:foo(1)
	`
	graph, bindings, call, point := parseGraphAndMethodCall(t, src)

	if !ForceMethodReceiver(bindings, graph, call) {
		t.Fatal("expected ForceMethodReceiver to resolve method symbol through alias receiver base")
	}
	if !ForceMethodReceiverAtPoint(bindings, graph, point, call.Call) {
		t.Fatal("expected ForceMethodReceiverAtPoint to resolve method symbol through alias receiver base")
	}
}

func parseGraphAndMethodCall(t *testing.T, src string) (*ccfg.Graph, *bind.BindingTable, *ccfg.CallInfo, typecfg.Point) {
	t.Helper()
	stmts, err := parse.Parse(strings.NewReader(src), "test")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{Stmts: stmts}
	bindings := bind.Bind(fn, nil)
	graph := ccfg.BuildWithBindings(fn, bindings)

	var (
		call  *ccfg.CallInfo
		point typecfg.Point
	)
	graph.EachCallSite(func(p typecfg.Point, info *ccfg.CallInfo) {
		if call != nil || info == nil || info.Method != "foo" {
			return
		}
		call = info
		point = p
	})
	if call == nil {
		t.Fatal("expected method call info")
	}
	return graph, bindings, call, point
}
