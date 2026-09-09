package flow

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
)

// TestSSAVisibility_RHSInferredNotDeclared verifies that RHS-inferred types
// do NOT appear in DeclaredTypes during Declared. Only annotated types should
// be in DeclaredTypes.
func TestSSAVisibility_RHSInferredNotDeclared(t *testing.T) {
	source := `
		local tbl = {name = "test", count = 42}
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected error: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	rootResult := result.Session.RootResult
	if rootResult == nil {
		t.Fatal("no root result")
	}

	inputs := rootResult.FlowInputs
	if inputs == nil {
		t.Fatal("no flow inputs")
	}

	// Find the symbol for 'tbl'
	var tblSym cfg.SymbolID
	graph := rootResult.Graph
	for _, p := range graph.RPO() {
		if sym, ok := graph.SymbolAt(p, "tbl"); ok {
			tblSym = sym
			break
		}
	}
	if tblSym == 0 {
		t.Fatal("could not find symbol for 'tbl'")
	}

	// DeclaredTypes should NOT contain 'tbl' since it has no annotation
	if declType, exists := inputs.DeclaredTypes[tblSym]; exists {
		t.Errorf("RHS-inferred type for non-annotated 'tbl' leaked into DeclaredTypes: %v", declType)
	}

	// AnnotatedVars should NOT contain 'tbl'
	if inputs.AnnotatedVars != nil && inputs.AnnotatedVars[tblSym] {
		t.Error("non-annotated 'tbl' incorrectly marked as annotated")
	}
}

// TestSSAVisibility_AnnotatedInDeclared verifies that annotated types DO appear
// in DeclaredTypes.
func TestSSAVisibility_AnnotatedInDeclared(t *testing.T) {
	source := `
		local tbl: {name: string, count: number} = {name = "test", count = 42}
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected error: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	rootResult := result.Session.RootResult
	if rootResult == nil {
		t.Fatal("no root result")
	}

	inputs := rootResult.FlowInputs
	if inputs == nil {
		t.Fatal("no flow inputs")
	}

	var tblSym cfg.SymbolID
	graph := rootResult.Graph
	for _, p := range graph.RPO() {
		if sym, ok := graph.SymbolAt(p, "tbl"); ok {
			tblSym = sym
			break
		}
	}
	if tblSym == 0 {
		t.Fatal("could not find symbol for 'tbl'")
	}

	// DeclaredTypes SHOULD contain 'tbl' since it has an annotation
	if _, exists := inputs.DeclaredTypes[tblSym]; !exists {
		t.Error("annotated 'tbl' should be in DeclaredTypes")
	}

	// AnnotatedVars SHOULD contain 'tbl'
	if inputs.AnnotatedVars == nil || !inputs.AnnotatedVars[tblSym] {
		t.Error("annotated 'tbl' should be in AnnotatedVars")
	}
}

// TestSSAVisibility_NestedFuncBeforeAssignment verifies that a nested function
// defined BEFORE a variable assignment does NOT see the later assignment's type.
// This tests proper SSA version visibility.
func TestSSAVisibility_NestedFuncBeforeAssignment(t *testing.T) {
	source := `
		local x

		local function before_assign(): any
			return x
		end

		x = {name = "test", count = 42}

		local n: string = x.name
	`

	result := testutil.Check(source, testutil.WithStdlib())
	// The test should pass type checking because:
	// - before_assign returns 'any' (x is unknown at that point)
	// - After assignment, x has the record type
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestSSAVisibility_NestedFuncAfterAssignment verifies that a nested function
// defined AFTER a variable assignment DOES see the assigned type.
func TestSSAVisibility_NestedFuncAfterAssignment(t *testing.T) {
	source := `
		local x = {name = "test", count = 42}

		local function after_assign(): string
			return x.name
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	// This should pass because the nested function is defined AFTER the assignment
	// and should see the record type for x
	if result.HasError() {
		t.Fatalf("nested function after assignment should see enriched type: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestSSAVisibility_NestedFuncSeesWrongVersion verifies that a nested function
// defined BEFORE an assignment does NOT see the later assignment's type.
// The return type mismatch is the true indicator of the bug.
func TestSSAVisibility_NestedFuncSeesWrongVersion(t *testing.T) {
	// If the nested function sees x as Unknown, returning x.name as string should fail
	// because x.name is Unknown, not string.
	// But field access on Unknown is silently skipped, so we need a different approach.

	// Instead, we verify that the return value type mismatches.
	// If x is Unknown, then x.name is also Unknown, and assigning Unknown to string should fail.
	source := `
		local x

		local function f(): string
			local result: string = x.name
			return result
		end

		x = {name = "test"}
	`

	result := testutil.Check(source, testutil.WithStdlib())
	// If the bug exists (nested func sees later assignment), x.name is string, assignment works.
	// If the bug is fixed, x.name is Unknown, assignment to string fails.
	if !result.HasError() {
		t.Error("nested function before assignment should NOT see later assignment's type - assigning Unknown to string should fail")
	}
}

// TestSSAVisibility_NestedFuncScopeInspection traces the scope at nested function definition
func TestSSAVisibility_NestedFuncScopeInspection(t *testing.T) {
	source := `
		local x

		local function f()
			local y = x
		end

		x = {name = "test"}
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	rootResult := result.Session.RootResult
	if rootResult == nil || rootResult.Graph == nil {
		t.Fatal("no root result")
	}

	graph := rootResult.Graph

	// Find the nested function definition point
	nestedFuncs := graph.NestedFunctions()
	if len(nestedFuncs) == 0 {
		t.Fatal("no nested functions found")
	}

	nf := nestedFuncs[0]
	defPoint := nf.Point

	// Check what type x has at the nested function's definition point using TypeFacts
	sym, ok := graph.SymbolAt(defPoint, "x")
	if !ok {
		t.Fatal("x symbol not found at nested function definition point")
	}

	if rootResult.Facts == nil {
		t.Fatal("no type facts")
	}

	tv := rootResult.Facts.EffectiveTypeAt(defPoint, sym)

	// x should be unknown at the nested function definition point (before assignment)
	if tv.Type != nil && tv.Type.Kind() != kind.Unknown && tv.Type.Kind() != kind.Any {
		t.Errorf("x should be unknown at nested function definition point, got: %v (kind=%v)", tv.Type, tv.Type.Kind())
	}
}

// TestSSAVisibility_NestedFuncTypeFacts inspects what type the nested function actually sees for x
func TestSSAVisibility_NestedFuncTypeFacts(t *testing.T) {
	source := `
		local x

		local function f()
			local y = x
		end

		x = {name = "test"}
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	// Get the nested function's FuncResult
	for fn, funcResult := range result.Session.Results {
		if fn != result.Session.RootFunc {
			nestedResult := funcResult

			// Check the nested function's declared types for x
			if nestedResult.FlowInputs != nil {
				for sym, declType := range nestedResult.FlowInputs.DeclaredTypes {
					name := ""
					if nestedResult.Graph != nil {
						name = nestedResult.Graph.NameOf(sym)
					}
					if name == "x" {
						if declType != nil && declType.Kind() != kind.Unknown && declType.Kind() != kind.Any {
							t.Errorf("nested function's DeclaredTypes for x should be Unknown, got: %v (kind=%v)", declType, declType.Kind())
						} else {
							t.Logf("nested function's DeclaredTypes for x: %v (kind=%v) - correct!", declType, declType.Kind())
						}
						return
					}
				}
			}
			t.Log("x not found in nested function's DeclaredTypes (might be correct if not declared)")
			return
		}
	}
	t.Log("no nested function result found")
}

// TestSSAVisibility_ParentSymbolTypesAtNestedPoint checks symbolTypes at nested function point
func TestSSAVisibility_ParentSymbolTypesAtNestedPoint(t *testing.T) {
	source := `
		local x

		local function f()
			local y = x
		end

		x = {name = "test"}
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	rootResult := result.Session.RootResult
	if rootResult == nil || rootResult.Graph == nil {
		t.Fatal("no root result")
	}

	graph := rootResult.Graph
	nestedFuncs := graph.NestedFunctions()
	if len(nestedFuncs) == 0 {
		t.Fatal("no nested functions found")
	}

	nfPoint := nestedFuncs[0].Point

	// Check the parent's FlowInputs to see what symbolTypes had for x at the nested function point
	if rootResult.FlowInputs != nil {
		// DeclaredTypes is keyed by SymbolID, not point, but let's check what x's type is
		for sym, declType := range rootResult.FlowInputs.DeclaredTypes {
			name := graph.NameOf(sym)
			if name == "x" {
				t.Logf("parent's DeclaredTypes for x: %v (kind=%v)", declType, declType.Kind())
				// x should NOT have a record type in DeclaredTypes if it's not annotated
				if declType != nil && declType.Kind() == kind.Record {
					t.Errorf("x should NOT have record type in parent's DeclaredTypes (RHS inference leaked)")
				}
			}
		}
	}

	// Check if x is visible at the nested function's definition point
	xSym, ok := graph.SymbolAt(nfPoint, "x")
	if !ok {
		t.Log("x not visible at nested function point")
		return
	}

	// Check the SSA version of x at different points
	xVerAtNF := graph.VisibleVersion(nfPoint, xSym)
	t.Logf("x SSA version at nested function point: %v", xVerAtNF)

	// Find the assignment point and check x's version there
	for _, p := range graph.RPO() {
		info := graph.Assign(p)
		if info != nil && len(info.Targets) > 0 {
			for _, target := range info.Targets {
				if target.Name == "x" && len(info.Sources) > 0 {
					xVerAtAssign := graph.VisibleVersion(p, xSym)
					t.Logf("x SSA version at assignment point %d: %v", p, xVerAtAssign)
					if xVerAtNF == xVerAtAssign {
						t.Errorf("x's SSA version at nested function point should differ from assignment point")
					}
				}
			}
		}
	}
}

// TestSSAVisibility_EnrichmentOnlyAtVisiblePoints tests that RHS enrichment
// only affects points where the SSA version is visible.
func TestSSAVisibility_EnrichmentOnlyAtVisiblePoints(t *testing.T) {
	source := `
		local x

		-- At this point, x should be unknown
		local y = x

		x = {name = "test"}

		-- At this point, x should have the record type
		local n: string = x.name
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	rootResult := result.Session.RootResult
	if rootResult == nil || rootResult.FlowSolution == nil {
		t.Fatal("no flow solution")
	}

	// Verify that 'y' is unknown (not enriched with x's later type)
	graph := rootResult.Graph
	solution := rootResult.FlowSolution

	for _, p := range graph.RPO() {
		if sym, ok := graph.SymbolAt(p, "y"); ok {
			path := constraint.Path{Root: "y", Symbol: sym}
			yType := solution.TypeAt(p, path)
			if yType != nil && yType.Kind() != kind.Unknown && yType.Kind() != kind.Any {
				t.Fatalf("expected y to be unknown or any, got %v", yType)
			}
			break
		}
	}
}
