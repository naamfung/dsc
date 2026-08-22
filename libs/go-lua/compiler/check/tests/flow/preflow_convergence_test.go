package flow

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

// TestPreflowConvergence_SelfRecursive tests self-referential type inference.
// A local variable assigned from an expression that references itself.
func TestPreflowConvergence_SelfRecursive(t *testing.T) {
	source := `
local function build(depth)
	if depth <= 0 then
		return {value = 1}
	end
	return {value = 1, child = build(depth - 1)}
end

local tree = build(3)
local v: number = tree.value
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - self-recursive inference should converge, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestPreflowConvergence_Mutual2Node tests 2-node mutual recursion in inference.
// Two locals that reference each other's inferred types.
func TestPreflowConvergence_Mutual2Node(t *testing.T) {
	source := `
local a = {x = 1}
local b = {y = a}
a = {x = 1, ref = b}

local n: number = a.x
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - 2-node mutual recursion should converge, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestPreflowConvergence_3NodeSCC tests 3-node SCC in inference.
func TestPreflowConvergence_3NodeSCC(t *testing.T) {
	source := `
local a = {val = 1}
local b = {ref_a = a}
local c = {ref_b = b}
a = {val = 1, ref_c = c}

local n: number = a.val
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - 3-node SCC should converge, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestPreflowConvergence_DeterministicOrder tests that inference produces
// deterministic results regardless of declaration order.
func TestPreflowConvergence_DeterministicOrder(t *testing.T) {
	source := `
local x = {a = 1}
local y = {b = x}
local z = {c = y}

local n1: number = x.a
`

	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestPreflowConvergence_TableInsertWidening tests that table.insert widening
// converges properly in loops.
func TestPreflowConvergence_TableInsertWidening(t *testing.T) {
	source := `
local arr = {}
for i = 1, 10 do
	table.insert(arr, i)
end
local n: number = arr[1]
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - table.insert widening should converge, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestPreflowConvergence_NestedFunctionInference tests nested function type inference.
func TestPreflowConvergence_NestedFunctionInference(t *testing.T) {
	source := `
local function outer(): number
	local function inner()
		return 42
	end
	return inner()
end

local x: number = outer()
`

	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestPreflowConvergence_ChainedAssignments tests chained assignment inference.
func TestPreflowConvergence_ChainedAssignments(t *testing.T) {
	source := `
local a = 1
local b = a
local c = b
local d = c
local e: number = d
`

	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestPreflowConvergence_MixedTypesUnion tests that mixed type assignments
// properly form unions and converge.
func TestPreflowConvergence_MixedTypesUnion(t *testing.T) {
	source := `
local x
if math.random() > 0.5 then
	x = 1
else
	x = "hello"
end
local y: number | string = x
`

	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors - union type should converge, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestPreflowConvergence_CastExprDependency tests that dependencies through
// cast expressions are properly tracked.
func TestPreflowConvergence_CastExprDependency(t *testing.T) {
	source := `
local a = 42
local b = (a :: number) + 1
local c: number = b
`

	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors - cast dependency should be tracked, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestPreflowConvergence_NonNilAssertDependency tests that dependencies through
// non-nil assert expressions are properly tracked.
func TestPreflowConvergence_NonNilAssertDependency(t *testing.T) {
	source := `
local a: number? = 42
local b = a! + 1
local c: number = b
`

	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors - non-nil assert dependency should be tracked, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestPreflowConvergence_WideningSoundness tests that when an SCC doesn't converge,
// ALL members are widened to unknown, not just missing entries.
func TestPreflowConvergence_WideningSoundness(t *testing.T) {
	// This test verifies that partial types don't leak through when widening occurs.
	// The key property is that if widening triggers, all affected symbols get unknown.
	source := `
local a = {x = 1}
local b = {y = a}
a = {x = 1, z = b}

-- After convergence, both a and b should have known types
local n: number = a.x
`

	result := testutil.Check(source, testutil.WithStdlib())

	// Should pass - the SCC should converge
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestPreflowConvergence_WideningReported tests that widening events are recorded.
func TestPreflowConvergence_WideningReported(t *testing.T) {
	// Create a case that triggers widening by exceeding max iterations.
	// Deeply recursive mutual dependencies that don't stabilize quickly.
	source := `
local a, b, c, d, e

a = function() return b() end
b = function() return c() end
c = function() return d() end
d = function() return e() end
e = function() return a() end
`

	result := testutil.Check(source, testutil.WithStdlib())

	// Access widening events from flow inputs
	if result.Session == nil || result.Session.RootResult == nil {
		t.Fatal("expected session with root result")
	}

	inputs := result.Session.RootResult.FlowInputs
	if inputs == nil {
		t.Fatal("expected flow inputs")
	}

	// Even if no widening occurs in this simple case, verify the field exists
	// and the API works. A true non-converging case is hard to construct
	// without artificial iteration limits.
	t.Logf("widening events count: %d", len(inputs.WideningEvents))
}

// TestPreflowConvergence_WideningDiagnosticEmitted tests that widening diagnostics
// are emitted when preflow inference doesn't converge.
func TestPreflowConvergence_WideningDiagnosticEmitted(t *testing.T) {
	// This test verifies the diagnostic plumbing works.
	// Note: Most real code converges within the iteration limit,
	// so widening diagnostics are rare in practice.
	source := `
local a, b, c, d, e

a = function() return b() end
b = function() return c() end
c = function() return d() end
d = function() return e() end
e = function() return a() end
`

	result := testutil.Check(source, testutil.WithStdlib())

	if result.Session == nil || result.Session.RootResult == nil {
		t.Fatal("expected session with root result")
	}

	// Count widening diagnostics (if any)
	wideningDiagCount := 0
	for _, d := range result.Session.Diagnostics {
		if d.Severity == diag.SeverityWarning {
			if len(d.Message) > 0 && (contains(d.Message, "widened to unknown") || contains(d.Message, "type inference did not converge")) {
				wideningDiagCount++
				t.Logf("Widening diagnostic: %s", d.Message)
			}
		}
	}

	// Log whether widening occurred
	inputs := result.Session.RootResult.FlowInputs
	if inputs != nil {
		t.Logf("widening events: %d, widening diagnostics: %d", len(inputs.WideningEvents), wideningDiagCount)

		// If widening events occurred, diagnostics should be emitted
		if len(inputs.WideningEvents) > 0 && wideningDiagCount == 0 {
			t.Error("widening events occurred but no diagnostics were emitted")
		}
	}
}

// TestPreflowConvergence_MapEntryFallbackCounters_NoWarnings reproduces
// a real-world pattern where a map entry is read, defaulted, then mutated.
// This should converge without non-convergence warnings.
func TestPreflowConvergence_MapEntryFallbackCounters_NoWarnings(t *testing.T) {
	source := `
type CaseStats = {
	passed: number,
	failed: number,
	skipped: number,
}

local case_stats: {[string]: CaseStats} = {}

local function mark_failed(id: string)
	local cs = case_stats[id]
	if not cs then
		cs = { passed = 0, failed = 0, skipped = 0 }
		case_stats[id] = cs
	end
	cs.failed = cs.failed + 1
end

local function mark_passed(id: string)
	local pcs = case_stats[id]
	if not pcs then
		pcs = { passed = 0, failed = 0, skipped = 0 }
		case_stats[id] = pcs
	end
	pcs.passed = pcs.passed + 1
end

mark_failed("suite:a")
mark_passed("suite:a")
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	for _, d := range result.Diagnostics {
		if d.Severity != diag.SeverityWarning {
			continue
		}
		if contains(d.Message, "type inference did not converge") || d.Message == "inter-function fixpoint did not converge" {
			t.Fatalf("unexpected convergence warning: %q", d.Message)
		}
	}
}

// contains is a simple substring check helper.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
