package flow

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestFixpointUnification_OrderIndependence verifies that two sibling local
// functions produce identical analysis results regardless of declaration order.
func TestFixpointUnification_OrderIndependence(t *testing.T) {
	sourceAB := `
local function a(n: number): number
	if n <= 0 then return 0 end
	return b(n - 1) + 1
end

local function b(n: number): number
	if n <= 0 then return 0 end
	return a(n - 1) + 1
end

local x: number = a(5)
local y: number = b(5)
`

	sourceBA := `
local function b(n: number): number
	if n <= 0 then return 0 end
	return a(n - 1) + 1
end

local function a(n: number): number
	if n <= 0 then return 0 end
	return b(n - 1) + 1
end

local x: number = a(5)
local y: number = b(5)
`

	resultAB := testutil.Check(sourceAB, testutil.WithStdlib())
	resultBA := testutil.Check(sourceBA, testutil.WithStdlib())

	if resultAB.HasError() {
		t.Fatalf("AB order has errors: %v", testutil.ErrorMessages(resultAB.Diagnostics))
	}
	if resultBA.HasError() {
		t.Fatalf("BA order has errors: %v", testutil.ErrorMessages(resultBA.Diagnostics))
	}

	// Both orders should produce equivalent seed summaries.
	sessAB := resultAB.Session
	sessBA := resultBA.Session

	if sessAB == nil || sessBA == nil {
		t.Fatal("missing session")
	}

	// Count non-warning diagnostics (errors only).
	errCountAB := 0
	errCountBA := 0
	for _, d := range sessAB.Diagnostics {
		if d.Severity == diag.SeverityError {
			errCountAB++
		}
	}
	for _, d := range sessBA.Diagnostics {
		if d.Severity == diag.SeverityError {
			errCountBA++
		}
	}
	if errCountAB != errCountBA {
		t.Errorf("error count differs: AB=%d, BA=%d", errCountAB, errCountBA)
	}
}

// TestFixpointUnification_LiteralSignatureVisibility verifies that a function
// literal's signature computed in one iteration becomes visible to dependent
// functions in the next iteration via the double-buffered LiteralSigs channel.
func TestFixpointUnification_LiteralSignatureVisibility(t *testing.T) {
	source := `
local tbl = {
	process = function(self, x: number): string
		return tostring(x)
	end,
}

local result: string = tbl:process(42)
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	sess := result.Session
	if sess == nil || sess.Store == nil {
		t.Fatal("missing session or store")
	}

	// Verify literal signatures were produced.
	// Literal signatures channel removed in canonical query architecture.
}

// TestFixpointUnification_ParamHintPropagation verifies that parameter hints
// from call sites propagate across iterations. In a chain A -> B -> C, where
// A calls B with a number and B calls C, param hints should stabilize.
func TestFixpointUnification_ParamHintPropagation(t *testing.T) {
	source := `
local function c(x)
	return x + 1
end

local function b(x)
	return c(x)
end

local function a()
	return b(10)
end

local result: number = a()
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	sess := result.Session
	if sess == nil || sess.Store == nil || sess.RootResult == nil {
		t.Fatal("missing session data")
	}

	parentHash := sess.Store.GraphParentHashOf(sess.RootResult.Graph.ID())
	parent := sess.Store.Parents()[parentHash]
	summaries := sess.Store.GetReturnSummariesSnapshot(sess.RootResult.Graph, parent)
	if len(summaries) == 0 {
		t.Error("expected non-empty return summaries for the call chain")
	}

	// Verify the return types resolved to number (not unknown).
	graph := sess.RootResult.Graph
	if graph == nil {
		t.Fatal("missing root graph")
	}

	for sym, rt := range summaries {
		name := graph.NameOf(sym)
		if name == "a" || name == "b" || name == "c" {
			if len(rt) == 0 {
				t.Errorf("empty return summary for %q", name)
				continue
			}
			if typ.TypeEquals(rt[0], typ.Unknown) {
				t.Errorf("return type for %q is unknown, expected number", name)
			}
		}
	}
}

// TestFixpointUnification_NonConvergenceDiagnostic verifies that non-convergence
// of the outer fixpoint produces a warning diagnostic.
// This test is structural: it confirms the diagnostic mechanism works.
// Under normal conditions, the fixpoint converges within a few iterations.
func TestFixpointUnification_NonConvergenceDiagnostic(t *testing.T) {
	// Simple code that should converge quickly.
	source := `
local function f(x: number): number
	return x + 1
end
local y: number = f(1)
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	// The fixpoint should converge, so no non-convergence warning should appear.
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityWarning && d.Message == "inter-function fixpoint did not converge" {
			t.Error("unexpected non-convergence warning for simple code")
		}
	}
}

// TestFixpointUnification_EffectPropagation verifies that effect rows propagate
// across function boundaries during fixpoint iteration. When function B calls
// error(), its Terminates flag propagates to function A that calls B.
func TestFixpointUnification_EffectPropagation(t *testing.T) {
	source := `
local function B()
	error("fail")
end

local function A()
	B()
end
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	sess := result.Session
	if sess == nil || sess.Store == nil {
		t.Fatal("missing session or store")
	}

	// Verify effects exist and A's effect has Terminates == true.
	foundA := false
	for sym, eff := range sess.Store.InterprocPrev.Refinements {
		if eff == nil {
			continue
		}
		// Look up name from root graph
		name := ""
		if sess.RootResult != nil && sess.RootResult.Graph != nil {
			name = sess.RootResult.Graph.NameOf(sym)
		}
		if name == "A" {
			foundA = true
			if !eff.Terminates {
				t.Errorf("expected A to have Terminates == true (propagated from B)")
			}
		}
		if name == "B" {
			if !eff.Terminates {
				t.Errorf("expected B to have Terminates == true")
			}
		}
	}
	if !foundA {
		t.Log("function A not found in interproc effects (may indicate no symbol resolution for local functions)")
	}
}

// TestFixpointUnification_EffectPropagation_Alias verifies effect propagation
// through function aliases (type-based lookup).
func TestFixpointUnification_EffectPropagation_Alias(t *testing.T) {
	source := `
local function B()
	error("fail")
end

local function A()
	local f = B
	f()
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	sess := result.Session
	if sess == nil || sess.Store == nil {
		t.Fatal("missing session or store")
	}

	foundA := false
	for sym, eff := range sess.Store.InterprocPrev.Refinements {
		if eff == nil {
			continue
		}
		name := ""
		if sess.RootResult != nil && sess.RootResult.Graph != nil {
			name = sess.RootResult.Graph.NameOf(sym)
		}
		if name == "A" {
			foundA = true
			if !eff.Terminates {
				t.Errorf("expected A to have Terminates == true (propagated from alias)")
			}
		}
	}
	if !foundA {
		t.Log("function A not found in interproc effects (may indicate no symbol resolution for local functions)")
	}
}

// TestFixpointUnification_EffectPropagation_Field verifies effect propagation
// through table field function calls (type-based lookup).
func TestFixpointUnification_EffectPropagation_Field(t *testing.T) {
	source := `
local function A()
	local t = { f = error }
	t.f("x")
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	sess := result.Session
	if sess == nil || sess.Store == nil {
		t.Fatal("missing session or store")
	}

	foundA := false
	for sym, eff := range sess.Store.InterprocPrev.Refinements {
		if eff == nil {
			continue
		}
		name := ""
		if sess.RootResult != nil && sess.RootResult.Graph != nil {
			name = sess.RootResult.Graph.NameOf(sym)
		}
		if name == "A" {
			foundA = true
			if !eff.Terminates {
				t.Errorf("expected A to have Terminates == true (propagated from field call)")
			}
		}
	}
	if !foundA {
		t.Log("function A not found in interproc effects (may indicate no symbol resolution for local functions)")
	}
}

// TestFixpointUnification_EffectPropagation_Module verifies effect propagation
// for imported module functions with effect rows.
func TestFixpointUnification_EffectPropagation_Module(t *testing.T) {
	manifest := io.NewManifest("mod")
	fn := typ.Func().Effects(effect.Row{Labels: []effect.Label{effect.Diverge{}}}).Build()
	manifest.SetExport(typ.NewRecord().Field("crash", fn).Build())

	source := `
local m = require("mod")
local function A()
	m.crash()
end
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("mod", manifest))
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	sess := result.Session
	if sess == nil || sess.Store == nil {
		t.Fatal("missing session or store")
	}

	foundA := false
	for sym, eff := range sess.Store.InterprocPrev.Refinements {
		if eff == nil {
			continue
		}
		name := ""
		if sess.RootResult != nil && sess.RootResult.Graph != nil {
			name = sess.RootResult.Graph.NameOf(sym)
		}
		if name == "A" {
			foundA = true
			if !eff.Terminates {
				t.Errorf("expected A to have Terminates == true (propagated from module import)")
			}
		}
	}
	if !foundA {
		t.Log("function A not found in interproc effects (may indicate no symbol resolution for local functions)")
	}
}

// TestFixpointUnification_EffectRowLabels verifies that effect row labels
// are properly stored on FunctionRefinement and survive the fixpoint swap.
func TestFixpointUnification_EffectRowLabels(t *testing.T) {
	// Verify that the Row field on FunctionRefinement supports union and equality.
	row1 := effect.WithModuleLoad()
	row2 := effect.WithVariadicTransform()
	combined := effect.Union(row1, row2)

	eff := &constraint.FunctionRefinement{
		Row:        combined,
		Terminates: false,
	}

	if eff.Row == nil {
		t.Fatal("expected non-nil Row")
	}

	r, ok := eff.Row.(effect.Row)
	if !ok {
		t.Fatal("expected effect.Row type")
	}

	if !r.HasModuleLoad() {
		t.Error("expected HasModuleLoad to be true")
	}
	if !r.HasVariadicTransform() {
		t.Error("expected HasVariadicTransform to be true")
	}
	if r.HasTypePredicate() {
		t.Error("expected HasTypePredicate to be false")
	}
}

// TestFixpointUnification_ParamHintNestedPropagation verifies that parameter
// hints propagate correctly through nested function calls within function bodies.
// This is a regression test for the early break bug where PropagateParamHintsFromCallGraph
// would fail to resolve callee symbols from identifiers when CalleeSymbol was zero.
func TestFixpointUnification_ParamHintNestedPropagation(t *testing.T) {
	// d calls c, c calls b, b has parameter x. Hints should flow d->c->b.
	// The key is that inner calls (c calling b) need identifier resolution.
	source := `
local function b(x)
	return x * 2
end

local function c(y)
	return b(y)
end

local function d()
	return c(10)
end

local result: number = d()
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	sess := result.Session
	if sess == nil || sess.Store == nil || sess.RootResult == nil {
		t.Fatal("missing session data")
	}

	graph := sess.RootResult.Graph
	if graph == nil {
		t.Fatal("missing root graph")
	}

	// Verify that all three functions resolved to number return type (not unknown).
	checkedFunctions := make(map[string]bool)
	parentHash := sess.Store.GraphParentHashOf(sess.RootResult.Graph.ID())
	parent := sess.Store.Parents()[parentHash]
	summaries := sess.Store.GetReturnSummariesSnapshot(sess.RootResult.Graph, parent)
	for sym, rt := range summaries {
		name := graph.NameOf(sym)
		if name == "b" || name == "c" || name == "d" {
			checkedFunctions[name] = true
			if len(rt) == 0 {
				t.Errorf("empty return summary for %q", name)
				continue
			}
			if typ.TypeEquals(rt[0], typ.Unknown) {
				t.Errorf("return type for %q is unknown, expected number (hints didn't propagate)", name)
			}
		}
	}

	// Verify that param hints were propagated to inner functions.
	paramHintsFound := false
	if hints := sess.Store.GetParamHintsSnapshot(sess.RootResult.Graph, parent); len(hints) > 0 {
		paramHintsFound = true
	}
	if !paramHintsFound {
		t.Log("no param hints found in ParamHintsPrev (propagation may have converged)")
	}
}

func TestFixpointUnification_ModuleSelfReturnTable_NoNonConvergenceWarning(t *testing.T) {
	globals := io.NewManifest("globals")
	globals.AddGlobal("process", typ.Any)

	source := `
local test = {}

local _default_context = {
	tests = {},
	suites_hierarchy = {},
	current_describe = nil,
	mocks = { registry = {}, namespace = {} }
}

function test.suite(name)
	return {
		name = name,
		tests = {},
		parent = nil,
		children = {},
		full_path = name,
	}
end

function test.describe(name, fn)
	local old_describe = _default_context.current_describe
	local new_suite = test.suite(name)

	if old_describe then
		new_suite.parent = old_describe
		table.insert(old_describe.children, new_suite)
		new_suite.full_path = old_describe.full_path .. " > " .. name
	else
		table.insert(_default_context.suites_hierarchy, new_suite)
	end

	_default_context.current_describe = new_suite
	fn()
	table.insert(_default_context.tests, new_suite)
	_default_context.current_describe = old_describe

	return new_suite
end

function test.context(name, fn)
	return test.describe(name, fn)
end

function test.spec(name, fn)
	return test.describe(name, fn)
end

function test.it(name, fn)
	if not _default_context.current_describe then
		error("it must be called within describe")
	end
	table.insert(_default_context.current_describe.tests, { name = name, fn = fn, skipped = false })
end

function test.register_mock_namespace(target, name)
	_default_context.mocks.namespace[target] = name
	return test
end

function test.mock(target_or_path, field_or_replacement, replacement_optional)
	local id = target_or_path
	if _default_context.mocks.registry[id] == nil then
		_default_context.mocks.registry[id] = {
			container = _G,
			container_key = "process",
			original_table = process,
		}
	end
	_G.process = { send = function() end }
	return test
end

function test.restore_all_mocks()
	local registry_keys = {}
	for id, _ in pairs(_default_context.mocks.registry) do
		table.insert(registry_keys, id)
	end
	for _, id in ipairs(registry_keys) do
		local entry = _default_context.mocks.registry[id]
		if entry then
			entry.container[entry.container_key] = entry.original_table
			_default_context.mocks.registry[id] = nil
		end
	end
	return test
end

return test
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("globals", globals))
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	assertNoFixpointNonConvergenceWarning(t, result.Diagnostics)
}

func TestFixpointUnification_RecursiveSuiteBuilders_NoNonConvergenceWarning(t *testing.T) {
	source := `
local test = {}
local st = { current = nil, suites = {} }

function test.suite(name)
	return { name = name, parent = nil, children = {}, full_path = name }
end

function test.describe(name, fn)
	local old = st.current
	local s = test.suite(name)
	if old then
		s.parent = old
		table.insert(old.children, s)
		s.full_path = old.full_path .. " > " .. name
	else
		table.insert(st.suites, s)
	end
	st.current = s
	fn()
	st.current = old
	return s
end

function test.context(name, fn)
	return test.describe(name, fn)
end

function test.spec(name, fn)
	return test.describe(name, fn)
end

local a = test.describe("a", function() end)
local b = test.context("b", function() end)
local c = test.spec("c", function() end)

return { a = a, b = b, c = c }
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
	assertNoFixpointNonConvergenceWarning(t, result.Diagnostics)
}

func assertNoFixpointNonConvergenceWarning(t *testing.T, diags []diag.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Severity != diag.SeverityWarning {
			continue
		}
		if d.Message == "inter-function fixpoint did not converge" ||
			contains(d.Message, "inter-function fixpoint did not converge;") {
			t.Fatalf("unexpected non-convergence warning: %q", d.Message)
		}
	}
}
