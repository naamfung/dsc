package flowbuild_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// testSynthAPI wraps a function to satisfy api.SynthAPI for tests.
type testSynthAPI struct {
	typeOf                func(ast.Expr, cfg.Point) typ.Type
	expandValuesWithSpec  func([]ast.Expr, int, cfg.Point, api.SpecTypes) []typ.Type
	inferIterVarsWithSpec func([]ast.Expr, int, cfg.Point, api.SpecTypes) []typ.Type
}

func (t *testSynthAPI) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	if t.typeOf != nil {
		return t.typeOf(expr, p)
	}
	return nil
}

func (t *testSynthAPI) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	return t.ExpandValuesWithSpecTypes(exprs, needed, p, nil)
}

func (t *testSynthAPI) InferIterVars(exprs []ast.Expr, count int, p cfg.Point) []typ.Type {
	return t.InferIterVarsWithSpecTypes(exprs, count, p, nil)
}

func (t *testSynthAPI) ExpandValuesWithSpecTypes(exprs []ast.Expr, needed int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	if t.expandValuesWithSpec != nil {
		return t.expandValuesWithSpec(exprs, needed, p, specTypes)
	}
	return nil
}

func (t *testSynthAPI) InferIterVarsWithSpecTypes(exprs []ast.Expr, count int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	if t.inferIterVarsWithSpec != nil {
		return t.inferIterVarsWithSpec(exprs, count, p, specTypes)
	}
	return nil
}

// buildTestContext creates a minimal Env for testing.
func buildTestContext(graph *cfg.Graph, base *scope.State) api.BaseEnv {
	return api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:     graph,
		BaseScope: base,
	})
}

// TestExtract_CallWithRefinement tests that constraints from function refinements are extracted.
func TestExtract_CallWithRefinement(t *testing.T) {
	code := `
assert_not_nil(x)
local y = x
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	// Seed global names into CFG so they get SymbolIDs
	graph := cfg.Build(fn, "assert_not_nil", "x")
	if graph == nil {
		t.Fatal("nil graph")
	}

	// Create assert_not_nil function with refinement
	notNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.NotNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)
	assertNotNil := typ.Func().
		Param("val", typ.Any).
		WithRefinement(notNilEffect).
		Build()

	// x is optional type
	xType := typ.NewUnion(typ.String, typ.Nil)

	// Look up seeded global SymbolIDs and build DeclaredTypes
	symAssertNotNil, okA := graph.SymbolAt(graph.Entry(), "assert_not_nil")
	symX, okX := graph.SymbolAt(graph.Entry(), "x")
	if !okA || !okX {
		t.Fatalf("globals not found: assert_not_nil=%v, x=%v", okA, okX)
	}

	declaredTypes := flow.DeclaredTypes{
		symAssertNotNil: assertNotNil,
		symX:            xType,
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})
	ctx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		DeclaredTypes: declaredTypes,
		BaseScope:     base,
	})

	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok {
			t.Logf("synthFunc called for IdentExpr: %s", ident.Value)
			// Look up via graph/context (uses DeclaredTypes)
			if sym, ok := graph.SymbolAt(p, ident.Value); ok {
				if tv := ctx.Types().EffectiveTypeAt(p, sym); tv.State == flow.StateResolved {
					if fn, ok := tv.Type.(*typ.Function); ok && fn.Refinement != nil {
						t.Logf("  -> found function with refinement")
					}
					return tv.Type
				}
			}
			t.Logf("  -> not found")
		} else {
			t.Logf("synthFunc called for non-IdentExpr: %T", expr)
		}
		return nil
	}

	// Log call info from graph before extraction
	t.Log("Calls in graph:")
	graph.EachStmtCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}
		t.Logf("  Point %v: CalleeName=%q Method=%q Args=%d Callee=%T ArgNames=%v",
			p, info.CalleeName, info.Method, len(info.Args), info.Callee, info.ArgNames)
	})

	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
	})

	if inputs == nil {
		t.Fatal("nil inputs")
	}

	t.Logf("EdgeConditions: %d", len(inputs.EdgeConditions))
	for i, ec := range inputs.EdgeConditions {
		t.Logf("  Edge %d: from=%v to=%v constraints=%v", i, ec.From, ec.To, ec.Condition.AllConstraints())
	}

	// Check that we have a NotNil constraint for x
	found := false
	for _, ec := range inputs.EdgeConditions {
		for _, c := range ec.Condition.AllConstraints() {
			if nn, ok := c.(constraint.NotNil); ok {
				t.Logf("Found NotNil constraint: path=%v", nn.Path)
				if nn.Path.Root == "x" {
					found = true
				}
			}
		}
	}

	if !found {
		t.Error("Expected NotNil(x) constraint after assert_not_nil(x) call")
	}
}

// TestExtract_ReassignmentWithConflictingConstraints tests that constraints
// are properly associated with the correct definition after reassignment.
func TestExtract_ReassignmentWithConflictingConstraints(t *testing.T) {
	code := `
local x = maybe_nil1()
assert_is_nil(x)
x = maybe_nil2()
assert_not_nil(x)
local y = x.field
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	// Seed global names into CFG so they get SymbolIDs
	graph := cfg.Build(fn, "assert_is_nil", "assert_not_nil", "maybe_nil1", "maybe_nil2")
	if graph == nil {
		t.Fatal("nil graph")
	}

	// Create assert_is_nil function with refinement
	isNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.IsNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)
	assertIsNil := typ.Func().
		Param("val", typ.Any).
		WithRefinement(isNilEffect).
		Build()

	// Create assert_not_nil function with refinement
	notNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.NotNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)
	assertNotNil := typ.Func().
		Param("val", typ.Any).
		WithRefinement(notNilEffect).
		Build()

	// Record type with a field
	recType := typ.NewRecord().Field("field", typ.Number).Build()

	// maybe_nil returns Record | nil
	maybeNil := typ.Func().Returns(typ.NewUnion(recType, typ.Nil)).Build()

	// Look up seeded global SymbolIDs and build DeclaredTypes
	symAssertIsNil, _ := graph.SymbolAt(graph.Entry(), "assert_is_nil")
	symAssertNotNil, _ := graph.SymbolAt(graph.Entry(), "assert_not_nil")
	symMaybeNil1, _ := graph.SymbolAt(graph.Entry(), "maybe_nil1")
	symMaybeNil2, _ := graph.SymbolAt(graph.Entry(), "maybe_nil2")

	declaredTypes := flow.DeclaredTypes{
		symAssertIsNil:  assertIsNil,
		symAssertNotNil: assertNotNil,
		symMaybeNil1:    maybeNil,
		symMaybeNil2:    maybeNil,
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})
	ctx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		DeclaredTypes: declaredTypes,
		BaseScope:     base,
	})

	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok {
			// Look up via graph/context (uses DeclaredTypes)
			if sym, ok := graph.SymbolAt(p, ident.Value); ok {
				if tv := ctx.Types().EffectiveTypeAt(p, sym); tv.State == flow.StateResolved {
					return tv.Type
				}
			}
		}
		return nil
	}

	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
	})

	if inputs == nil {
		t.Fatal("nil inputs")
	}

	t.Logf("EdgeConditions: %d", len(inputs.EdgeConditions))
	for i, ec := range inputs.EdgeConditions {
		t.Logf("  Edge %d: from=%v to=%v constraints=%v", i, ec.From, ec.To, ec.Condition.AllConstraints())
	}

	t.Logf("Assignments: %d", len(inputs.Assignments))
	for _, a := range inputs.Assignments {
		t.Logf("  Point=%v Target=%s Symbol=%v Type=%v", a.Point, a.TargetPath.Root, a.TargetPath.Symbol, a.Type)
	}

	// Check that we have both IsNil and NotNil constraints on different edges
	hasIsNil := false
	hasNotNil := false
	for _, ec := range inputs.EdgeConditions {
		for _, c := range ec.Condition.AllConstraints() {
			if _, ok := c.(constraint.IsNil); ok {
				hasIsNil = true
				t.Logf("Found IsNil constraint at edge %v->%v", ec.From, ec.To)
			}
			if _, ok := c.(constraint.NotNil); ok {
				hasNotNil = true
				t.Logf("Found NotNil constraint at edge %v->%v", ec.From, ec.To)
			}
		}
	}

	if !hasIsNil {
		t.Error("Expected IsNil constraint")
	}
	if !hasNotNil {
		t.Error("Expected NotNil constraint")
	}
}

func TestExtract_ModulePattern(t *testing.T) {
	// Module pattern without annotation - verifies non-annotated vars are NOT in DeclaredTypes
	code := `
local M = {}
function M.add(a, b)
	return a + b
end
local result = M.add(1, 2)
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("nil graph")
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})

	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type { return nil }

	ctx := buildTestContext(graph, base)
	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
	})

	if inputs == nil {
		t.Fatal("nil inputs")
	}

	// Look up "M" via SymbolAt to get its SymbolID (at exit where it's visible)
	symM, ok := graph.SymbolAt(graph.Exit(), "M")
	if !ok {
		t.Error("M not found in graph symbols")
		return
	}

	// M should NOT be in AnnotatedVars since it has no type annotation
	if inputs.AnnotatedVars != nil && inputs.AnnotatedVars[symM] {
		t.Error("M should NOT be in AnnotatedVars (no annotation)")
	}

	// M should NOT be in DeclaredTypes (only annotated vars are stored)
	mType := inputs.DeclaredTypes[symM]
	t.Logf("DeclaredTypes[M] = %v (%T)", mType, mType)

	if mType != nil {
		t.Error("M should NOT be in DeclaredTypes (no annotation)")
	}
}

// TestDeclaredTypes_AnnotatedLocal verifies that annotated locals are in DeclaredTypes.
func TestDeclaredTypes_AnnotatedLocal(t *testing.T) {
	code := `local x: number = 42`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("nil graph")
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})

	// Build declared types directly (x has type annotation: number)
	symX, ok := graph.SymbolAt(graph.Exit(), "x")
	if !ok {
		t.Fatal("x not found in graph symbols")
	}
	declaredTypes := flow.DeclaredTypes{
		symX: typ.Number,
	}
	annotatedVars := map[cfg.SymbolID]bool{
		symX: true,
	}

	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type { return nil }

	ctx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		DeclaredTypes: declaredTypes,
		AnnotatedVars: annotatedVars,
		BaseScope:     base,
	})
	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	// x should be in AnnotatedVars
	if inputs.AnnotatedVars == nil || !inputs.AnnotatedVars[symX] {
		t.Error("x should be in AnnotatedVars")
	}

	// x should be in DeclaredTypes with type number
	xType := inputs.DeclaredTypes[symX]
	if xType == nil {
		t.Fatal("x should be in DeclaredTypes")
	}
	if xType != typ.Number {
		t.Errorf("x should have type number, got %v", xType)
	}
}

// TestDeclaredTypes_UnannotatedLocal verifies unannotated locals are NOT in DeclaredTypes.
func TestDeclaredTypes_UnannotatedLocal(t *testing.T) {
	code := `local x = 42`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("nil graph")
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})
	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type { return nil }

	ctx := buildTestContext(graph, base)
	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	symX, ok := graph.SymbolAt(graph.Exit(), "x")
	if !ok {
		t.Fatal("x not found in graph symbols")
	}

	// x should NOT be in AnnotatedVars
	if inputs.AnnotatedVars != nil && inputs.AnnotatedVars[symX] {
		t.Error("x should NOT be in AnnotatedVars (no annotation)")
	}

	// x should NOT be in DeclaredTypes
	if inputs.DeclaredTypes[symX] != nil {
		t.Error("x should NOT be in DeclaredTypes (no annotation)")
	}
}

// TestDeclaredTypes_FunctionDefWithSignature verifies function definitions
// (using "function foo()" syntax) are in DeclaredTypes when FnSigResolver is provided.
// Note: "local function foo()" uses LocalAssignStmt and does NOT create FuncDefInfo.
// Only "function foo()" creates FuncDefInfo which is used for DeclaredTypes.
func TestDeclaredTypes_FunctionDefWithSignature(t *testing.T) {
	// Note: "function add()" creates FuncDefInfo, "local function add()" creates AssignInfo
	code := `
function add(a, b)
	return a + b
end
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("nil graph")
	}

	// FunctionSignatureResolver builds function type from AST
	fnSigResolver := func(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
		return typ.Func().
			Param("a", typ.Number).
			Param("b", typ.Number).
			Returns(typ.Number).
			Build()
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})
	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type { return nil }

	ctx := buildTestContext(graph, base)
	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
		Services: core.FlowServicesFuncs{FnSigResolver: fnSigResolver},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	symAdd, ok := graph.SymbolAt(graph.Exit(), "add")
	if !ok {
		t.Fatal("add not found in graph symbols")
	}

	// add should be in DeclaredTypes with function type
	addType := inputs.DeclaredTypes[symAdd]
	if addType == nil {
		t.Fatal("add should be in DeclaredTypes when FnSigResolver is provided")
	}
	if _, ok := addType.(*typ.Function); !ok {
		t.Errorf("add should have function type, got %T", addType)
	}
}

// TestDeclaredTypes_LocalFunctionWithSignature verifies "local function foo()" with
// explicit type annotations is in DeclaredTypes.
func TestDeclaredTypes_LocalFunctionWithSignature(t *testing.T) {
	// "local function add(a: number, b: number): number" has explicit annotations
	code := `
local function add(a: number, b: number): number
	return a + b
end
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("nil graph")
	}

	// FunctionSignatureResolver builds function type from AST annotations
	fnSigResolver := func(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
		// Check if function has annotations
		hasAnnotations := false
		for _, rt := range fn.ReturnTypes {
			if rt != nil {
				hasAnnotations = true
				break
			}
		}
		if fn.ParList != nil {
			for _, pt := range fn.ParList.Types {
				if pt != nil {
					hasAnnotations = true
					break
				}
			}
		}
		if !hasAnnotations {
			return nil
		}
		return typ.Func().
			Param("a", typ.Number).
			Param("b", typ.Number).
			Returns(typ.Number).
			Build()
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})
	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type { return nil }

	ctx := buildTestContext(graph, base)
	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
		Services: core.FlowServicesFuncs{FnSigResolver: fnSigResolver},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	symAdd, ok := graph.SymbolAt(graph.Exit(), "add")
	if !ok {
		t.Fatal("add not found in graph symbols")
	}

	// add should be in AnnotatedVars
	if inputs.AnnotatedVars == nil || !inputs.AnnotatedVars[symAdd] {
		t.Error("add should be in AnnotatedVars")
	}

	// add should be in DeclaredTypes with function type
	addType := inputs.DeclaredTypes[symAdd]
	if addType == nil {
		t.Fatal("add should be in DeclaredTypes (local function with annotations)")
	}
	if _, ok := addType.(*typ.Function); !ok {
		t.Errorf("add should have function type, got %T", addType)
	}
}

// TestDeclaredTypes_LocalFunctionWithoutSignature verifies "local function foo()" without
// explicit type annotations is NOT in DeclaredTypes.
func TestDeclaredTypes_LocalFunctionWithoutSignature(t *testing.T) {
	// "local function add(a, b)" has NO explicit annotations
	code := `
local function add(a, b)
	return a + b
end
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("nil graph")
	}

	// FunctionSignatureResolver returns nil for unannotated functions
	fnSigResolver := func(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
		hasAnnotations := false
		for _, rt := range fn.ReturnTypes {
			if rt != nil {
				hasAnnotations = true
				break
			}
		}
		if fn.ParList != nil {
			for _, pt := range fn.ParList.Types {
				if pt != nil {
					hasAnnotations = true
					break
				}
			}
		}
		if !hasAnnotations {
			return nil
		}
		return typ.Func().Build()
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})
	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type { return nil }

	ctx := buildTestContext(graph, base)
	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
		Services: core.FlowServicesFuncs{FnSigResolver: fnSigResolver},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	symAdd, ok := graph.SymbolAt(graph.Exit(), "add")
	if !ok {
		t.Fatal("add not found in graph symbols")
	}

	// add should NOT be in AnnotatedVars (no annotations)
	if inputs.AnnotatedVars != nil && inputs.AnnotatedVars[symAdd] {
		t.Error("add should NOT be in AnnotatedVars (no annotations)")
	}

	// add should NOT be in DeclaredTypes (no annotations)
	if inputs.DeclaredTypes[symAdd] != nil {
		t.Error("add should NOT be in DeclaredTypes (no annotations)")
	}
}

// TestDeclaredTypes_ModuleMethodNotInDeclaredTypes verifies that module methods
// (function M.foo()) are NOT stored in DeclaredTypes - they go to Assignments.
func TestDeclaredTypes_ModuleMethodNotInDeclaredTypes(t *testing.T) {
	code := `
local M = {}
function M.add(a, b)
	return a + b
end
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("nil graph")
	}

	fnSigResolver := func(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
		return typ.Func().
			Param("a", typ.Number).
			Param("b", typ.Number).
			Returns(typ.Number).
			Build()
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})

	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type {
		if fnExpr, ok := expr.(*ast.FunctionExpr); ok {
			return fnSigResolver(fnExpr, scopes[p])
		}
		return nil
	}

	ctx := buildTestContext(graph, base)
	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
		Services: core.FlowServicesFuncs{FnSigResolver: fnSigResolver},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	// Module methods (FuncDefField) don't create standalone symbols
	// The method is accessed as M.add, not as a variable "add"
	// Verify that DeclaredTypes does NOT contain a symbol for the method itself

	// M should be found (the receiver)
	symM, ok := graph.SymbolAt(graph.Exit(), "M")
	if !ok {
		t.Fatal("M not found in graph symbols")
	}

	// M should NOT be in DeclaredTypes (no annotation on M)
	if inputs.DeclaredTypes[symM] != nil {
		t.Error("M should NOT be in DeclaredTypes (no annotation)")
	}

	// The method M.add should appear in Assignments as a sub-path assignment
	foundMethodAssignment := false
	for _, a := range inputs.Assignments {
		if a.TargetPath.Symbol == symM && len(a.TargetPath.Segments) > 0 {
			if a.TargetPath.Segments[0].Name == "add" {
				foundMethodAssignment = true
				if _, ok := a.Type.(*typ.Function); !ok {
					t.Errorf("M.add assignment should have function type, got %T", a.Type)
				}
			}
		}
	}
	if !foundMethodAssignment {
		t.Error("M.add should appear in Assignments as sub-path assignment")
	}
}

// TestDeclaredTypes_KeyedBySymbolID verifies all declared types are keyed by SymbolID.
func TestDeclaredTypes_KeyedBySymbolID(t *testing.T) {
	code := `
local x: number = 1
local y: string = "a"
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("nil graph")
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})

	// Build declared types directly (x: number, y: string)
	symX, okX := graph.SymbolAt(graph.Exit(), "x")
	symY, okY := graph.SymbolAt(graph.Exit(), "y")
	if !okX || !okY {
		t.Fatal("x or y not found in graph symbols")
	}

	declaredTypes := flow.DeclaredTypes{
		symX: typ.Number,
		symY: typ.String,
	}
	annotatedVars := map[cfg.SymbolID]bool{
		symX: true,
		symY: true,
	}

	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type { return nil }

	ctx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		DeclaredTypes: declaredTypes,
		AnnotatedVars: annotatedVars,
		BaseScope:     base,
	})
	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	// Symbols should be different (CFG identity)
	if symX == symY {
		t.Error("x and y should have different SymbolIDs")
	}

	// Both should be in DeclaredTypes keyed by their SymbolID
	if inputs.DeclaredTypes[symX] == nil {
		t.Error("x should be in DeclaredTypes keyed by SymbolID")
	}
	if inputs.DeclaredTypes[symY] == nil {
		t.Error("y should be in DeclaredTypes keyed by SymbolID")
	}

	// Types should be correct
	if inputs.DeclaredTypes[symX] != typ.Number {
		t.Errorf("x should have type number, got %v", inputs.DeclaredTypes[symX])
	}
	if inputs.DeclaredTypes[symY] != typ.String {
		t.Errorf("y should have type string, got %v", inputs.DeclaredTypes[symY])
	}
}

// TestMultiReturnExpansion verifies multi-return expansion uses CFG point and SymbolID.
func TestMultiReturnExpansion(t *testing.T) {
	code := `local q, r = divmod(10, 3)`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	// Seed global names into CFG so they get SymbolIDs
	graph := cfg.Build(fn, "divmod")
	if graph == nil {
		t.Fatal("nil graph")
	}

	divmodFn := typ.Func().
		Param("a", typ.Number).
		Param("b", typ.Number).
		Returns(typ.Integer, typ.Integer).
		Build()

	// Look up seeded global SymbolID and build DeclaredTypes
	symDivmod, _ := graph.SymbolAt(graph.Entry(), "divmod")
	declaredTypes := flow.DeclaredTypes{
		symDivmod: divmodFn,
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})
	ctx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		DeclaredTypes: declaredTypes,
		BaseScope:     base,
	})

	var expandCalled bool
	var expandPoint cfg.Point

	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok {
			// Look up via graph/context (uses DeclaredTypes)
			if sym, ok := graph.SymbolAt(p, ident.Value); ok {
				if tv := ctx.Types().EffectiveTypeAt(p, sym); tv.State == flow.StateResolved {
					return tv.Type
				}
			}
		}
		return nil
	}

	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API: &testSynthAPI{
			typeOf: synthFunc,
			expandValuesWithSpec: func(exprs []ast.Expr, count int, p cfg.Point, overlay api.SpecTypes) []typ.Type {
				expandCalled = true
				expandPoint = p
				return []typ.Type{typ.Integer, typ.Integer}
			},
		},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	if !expandCalled {
		t.Error("expandValues should have been called for multi-return")
	}
	if expandPoint == 0 {
		t.Error("expandValues should receive non-zero CFG point")
	}

	symQ, okQ := graph.SymbolAt(graph.Exit(), "q")
	symR, okR := graph.SymbolAt(graph.Exit(), "r")
	if !okQ || !okR {
		t.Fatal("q or r not found in graph symbols")
	}

	// Verify assignments use SymbolID
	var foundQ, foundR bool
	for _, a := range inputs.Assignments {
		if a.TargetPath.Symbol == symQ {
			foundQ = true
			if a.Type != typ.Integer {
				t.Errorf("q should have type integer, got %v", a.Type)
			}
		}
		if a.TargetPath.Symbol == symR {
			foundR = true
			if a.Type != typ.Integer {
				t.Errorf("r should have type integer, got %v", a.Type)
			}
		}
	}
	if !foundQ {
		t.Error("q assignment not found")
	}
	if !foundR {
		t.Error("r assignment not found")
	}

	// Verify sibling assignments track multi-return relationship
	if inputs.SiblingAssignments == nil {
		t.Fatal("SiblingAssignments should not be nil")
	}

	var verQ, verR int
	for _, a := range inputs.Assignments {
		switch a.TargetPath.Symbol {
		case symQ:
			verQ = graph.VisibleVersion(a.Point, symQ).ID
		case symR:
			verR = graph.VisibleVersion(a.Point, symR).ID
		}
	}
	if verQ == 0 || verR == 0 {
		t.Fatal("expected SSA versions for q and r")
	}

	siblingQ := inputs.SiblingAssignments[flow.SiblingKey{Symbol: symQ, VersionID: verQ}]
	siblingR := inputs.SiblingAssignments[flow.SiblingKey{Symbol: symR, VersionID: verR}]
	if siblingQ == nil || siblingR == nil {
		t.Error("both q and r should be in SiblingAssignments")
	}
	if siblingQ != siblingR {
		t.Error("q and r should reference the same SiblingAssignment")
	}
}

func TestMultiReturnExpansion_TrailingCallBuildsTailSiblingAssignments(t *testing.T) {
	code := `local a, q, r = 1, divmod(10, 3)`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn, "divmod")
	if graph == nil {
		t.Fatal("nil graph")
	}

	divmodFn := typ.Func().
		Param("a", typ.Number).
		Param("b", typ.Number).
		Returns(typ.Integer, typ.Integer).
		Build()

	symDivmod, _ := graph.SymbolAt(graph.Entry(), "divmod")
	declaredTypes := flow.DeclaredTypes{
		symDivmod: divmodFn,
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})
	ctx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		DeclaredTypes: declaredTypes,
		BaseScope:     base,
	})

	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok {
			if sym, ok := graph.SymbolAt(p, ident.Value); ok {
				if tv := ctx.Types().EffectiveTypeAt(p, sym); tv.State == flow.StateResolved {
					return tv.Type
				}
			}
		}
		return nil
	}

	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API: &testSynthAPI{
			typeOf: synthFunc,
			expandValuesWithSpec: func(exprs []ast.Expr, count int, p cfg.Point, overlay api.SpecTypes) []typ.Type {
				return []typ.Type{typ.Number, typ.Integer, typ.Integer}
			},
		},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	symA, okA := graph.SymbolAt(graph.Exit(), "a")
	symQ, okQ := graph.SymbolAt(graph.Exit(), "q")
	symR, okR := graph.SymbolAt(graph.Exit(), "r")
	if !okA || !okQ || !okR {
		t.Fatal("a, q, r not found in graph symbols")
	}

	var verA, verQ, verR int
	for _, a := range inputs.Assignments {
		switch a.TargetPath.Symbol {
		case symA:
			verA = graph.VisibleVersion(a.Point, symA).ID
		case symQ:
			verQ = graph.VisibleVersion(a.Point, symQ).ID
		case symR:
			verR = graph.VisibleVersion(a.Point, symR).ID
		}
	}
	if verA == 0 || verQ == 0 || verR == 0 {
		t.Fatal("expected SSA versions for a, q, r")
	}

	siblingA := inputs.SiblingAssignments[flow.SiblingKey{Symbol: symA, VersionID: verA}]
	if siblingA != nil {
		t.Fatal("a should not be part of trailing-call sibling assignment")
	}

	siblingQ := inputs.SiblingAssignments[flow.SiblingKey{Symbol: symQ, VersionID: verQ}]
	siblingR := inputs.SiblingAssignments[flow.SiblingKey{Symbol: symR, VersionID: verR}]
	if siblingQ == nil || siblingR == nil {
		t.Fatal("q and r should have sibling assignments for trailing call expansion")
	}
	if siblingQ != siblingR {
		t.Fatal("q and r should share the same trailing-call sibling assignment")
	}
}

// TestDiscardSlot verifies _ discard slots are handled correctly.
func TestDiscardSlot(t *testing.T) {
	code := `local _, value = get_pair()`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	// Seed global names into CFG so they get SymbolIDs
	graph := cfg.Build(fn, "get_pair")
	if graph == nil {
		t.Fatal("nil graph")
	}

	getPairFn := typ.Func().Returns(typ.String, typ.Number).Build()

	// Look up seeded global SymbolID and build DeclaredTypes
	symGetPair, _ := graph.SymbolAt(graph.Entry(), "get_pair")
	declaredTypes := flow.DeclaredTypes{
		symGetPair: getPairFn,
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})
	ctx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		DeclaredTypes: declaredTypes,
		BaseScope:     base,
	})

	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type { return nil }

	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API: &testSynthAPI{
			typeOf: synthFunc,
			expandValuesWithSpec: func(exprs []ast.Expr, count int, p cfg.Point, overlay api.SpecTypes) []typ.Type {
				return []typ.Type{typ.String, typ.Number}
			},
		},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	// value should be found with Number type
	symValue, ok := graph.SymbolAt(graph.Exit(), "value")
	if !ok {
		t.Fatal("value not found")
	}

	var foundValue bool
	for _, a := range inputs.Assignments {
		if a.TargetPath.Symbol == symValue {
			foundValue = true
			if a.Type != typ.Number {
				t.Errorf("value should have type number, got %v", a.Type)
			}
		}
	}
	if !foundValue {
		t.Error("value assignment not found")
	}
}

// TestLocalFunctionAsRHS verifies local x = f() where f is a local function.
func TestLocalFunctionAsRHS(t *testing.T) {
	code := `
local function getValue(): number
	return 42
end
local x = getValue()
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("nil graph")
	}

	fnSigResolver := func(fnExpr *ast.FunctionExpr, sc *scope.State) *typ.Function {
		for _, rt := range fnExpr.ReturnTypes {
			if rt != nil {
				return typ.Func().Returns(typ.Number).Build()
			}
		}
		return nil
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})

	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type {
		sc := scopes[p]
		if fnExpr, ok := expr.(*ast.FunctionExpr); ok {
			return fnSigResolver(fnExpr, sc)
		}
		return nil
	}

	ctx := buildTestContext(graph, base)
	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API: &testSynthAPI{
			typeOf: synthFunc,
			expandValuesWithSpec: func(exprs []ast.Expr, count int, p cfg.Point, overlay api.SpecTypes) []typ.Type {
				if len(exprs) > 0 {
					if _, ok := exprs[0].(*ast.FuncCallExpr); ok {
						return []typ.Type{typ.Number}
					}
				}
				return nil
			},
		},
		Services: core.FlowServicesFuncs{FnSigResolver: fnSigResolver},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	symX, ok := graph.SymbolAt(graph.Exit(), "x")
	if !ok {
		t.Fatal("x not found")
	}

	// x should have Number type from call expansion
	var foundX bool
	for _, a := range inputs.Assignments {
		if a.TargetPath.Symbol == symX {
			foundX = true
			if a.Type != typ.Number {
				t.Errorf("x should have type number, got %v", a.Type)
			}
		}
	}
	if !foundX {
		t.Error("x assignment not found")
	}

	// getValue should be in DeclaredTypes (annotated function)
	symGetValue, ok := graph.SymbolAt(graph.Exit(), "getValue")
	if !ok {
		t.Fatal("getValue not found")
	}
	if inputs.DeclaredTypes[symGetValue] == nil {
		t.Error("getValue should be in DeclaredTypes (annotated local function)")
	}
}

// TestSourcePathUsesSymbolID verifies that SourcePath resolves SymbolID correctly.
func TestSourcePathUsesSymbolID(t *testing.T) {
	code := `
local a = 1
local b = a
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("nil graph")
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})

	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type { return typ.Number }

	ctx := buildTestContext(graph, base)
	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	symA, okA := graph.SymbolAt(graph.Exit(), "a")
	symB, okB := graph.SymbolAt(graph.Exit(), "b")
	if !okA || !okB {
		t.Fatal("a or b not found")
	}

	// b's assignment should have SourcePath with a's SymbolID
	var foundBWithSource bool
	for _, a := range inputs.Assignments {
		if a.TargetPath.Symbol == symB {
			if a.SourcePath.Symbol == symA {
				foundBWithSource = true
			}
		}
	}
	if !foundBWithSource {
		t.Error("b's assignment should have SourcePath with a's SymbolID")
	}
}

// TestSpecNarrowingDoesNotMutateDeclaredTypes verifies spec-narrowed types
// do NOT flow into DeclaredTypes - they only affect Assignments.
func TestSpecNarrowingDoesNotMutateDeclaredTypes(t *testing.T) {
	code := `
local ch = listen({message = true})
local msg = ch:receive()
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   chunk,
	}

	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("nil graph")
	}

	base := scope.New()
	scopes := phase.ComputeScopes(graph, base, nil, phase.ScopeOptions{})
	synthFunc := func(expr ast.Expr, p cfg.Point) typ.Type { return nil }

	ctx := buildTestContext(graph, base)
	inputs := flowbuild.Run(&core.FlowContext{
		Graph: graph, Scopes: scopes,
		CheckCtx: ctx,
		Base:     base,
		API:      &testSynthAPI{typeOf: synthFunc},
	})
	if inputs == nil {
		t.Fatal("nil inputs")
	}

	symCh, ok := graph.SymbolAt(graph.Exit(), "ch")
	if !ok {
		t.Fatal("ch not found")
	}

	// ch should NOT be in DeclaredTypes (no annotation, spec-narrowing goes to Assignments)
	if inputs.DeclaredTypes[symCh] != nil {
		t.Error("ch should NOT be in DeclaredTypes (spec-narrowed types go to Assignments only)")
	}

	// ch should be in Assignments
	var foundCh bool
	for _, a := range inputs.Assignments {
		if a.TargetPath.Symbol == symCh {
			foundCh = true
		}
	}
	if !foundCh {
		t.Error("ch should be in Assignments")
	}
}
