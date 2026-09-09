package decl

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFlowContext_ZeroValue(t *testing.T) {
	var fc core.FlowContext
	if fc.Graph != nil {
		t.Error("zero value Graph should be nil")
	}
	if fc.CheckCtx != nil {
		t.Error("zero value CheckCtx should be nil")
	}
	if fc.Base != nil {
		t.Error("zero value Base should be nil")
	}
	if fc.Scopes != nil {
		t.Error("zero value Scopes should be nil")
	}
	if fc.Globals != nil {
		t.Error("zero value Globals should be nil")
	}
	if fc.Services != nil {
		t.Error("zero value Services should be nil")
	}
}

func TestExtractTypeKeys_NilBase(t *testing.T) {
	fc := &core.FlowContext{Base: nil}
	inputs := &flow.Inputs{
		TypeKeys: make(map[uint64]typ.Type),
	}
	ExtractTypeKeys(fc, inputs)
	if len(inputs.TypeKeys) != 0 {
		t.Error("nil base should produce no type keys")
	}
}

func TestExtractTypeKeys_IncludesFlowScopes(t *testing.T) {
	localPointType := typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build()

	localScope := scope.New().WithType("Point", localPointType)
	fc := &core.FlowContext{
		Base:   nil,
		Scopes: map[cfg.Point]*scope.State{cfg.Point(1): localScope},
	}
	inputs := &flow.Inputs{
		TypeKeys: make(map[uint64]typ.Type),
	}

	ExtractTypeKeys(fc, inputs)

	if got := inputs.TypeKeys[localPointType.Hash()]; got == nil {
		t.Fatalf("expected local scope type key for Point hash %d", localPointType.Hash())
	}
}

func TestAddTypeKey_NilInputs(t *testing.T) {
	AddTypeKey(nil, typ.String)
}

func TestAddTypeKey_NilType(t *testing.T) {
	inputs := &flow.Inputs{
		TypeKeys: make(map[uint64]typ.Type),
	}
	AddTypeKey(inputs, nil)
	if len(inputs.TypeKeys) != 0 {
		t.Error("nil type should not add type key")
	}
}

func TestAddTypeKey_BasicType(t *testing.T) {
	inputs := &flow.Inputs{
		TypeKeys: make(map[uint64]typ.Type),
	}
	AddTypeKey(inputs, typ.String)
	if len(inputs.TypeKeys) == 0 {
		t.Error("basic type should add type key")
	}
}

func TestAddTypeKey_AliasType(t *testing.T) {
	inputs := &flow.Inputs{
		TypeKeys: make(map[uint64]typ.Type),
	}
	alias := &typ.Alias{Name: "MyString", Target: typ.String}
	AddTypeKey(inputs, alias)
	if len(inputs.TypeKeys) == 0 {
		t.Error("alias type should add type key")
	}
}

func TestAddTypeKey_AliasNilTarget(t *testing.T) {
	inputs := &flow.Inputs{
		TypeKeys: make(map[uint64]typ.Type),
	}
	alias := &typ.Alias{Name: "MyString", Target: nil}
	AddTypeKey(inputs, alias)
}

func TestAddTypeKey_MetaType(t *testing.T) {
	inputs := &flow.Inputs{
		TypeKeys: make(map[uint64]typ.Type),
	}
	meta := &typ.Meta{Of: typ.String}
	AddTypeKey(inputs, meta)
	if len(inputs.TypeKeys) == 0 {
		t.Error("meta type should add type key")
	}
}

func TestExtractDeclaredTypes_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type),
	}
	ExtractDeclaredTypes(fc, inputs)
	if len(inputs.DeclaredTypes) != 0 {
		t.Error("nil graph should produce no declared types")
	}
}

func TestExtractDeclaredTypes_EmptyGlobals(t *testing.T) {
	fc := &core.FlowContext{
		Globals: make(map[string]typ.Type),
	}
	inputs := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type),
	}
	ExtractDeclaredTypes(fc, inputs)
	if len(inputs.DeclaredTypes) != 0 {
		t.Error("nil graph should produce no declared types even with empty globals")
	}
}

func TestExtractModuleAliases_NilGraph(t *testing.T) {
	fc := &core.FlowContext{}
	inputs := &flow.Inputs{
		ModuleAliases: make(map[cfg.SymbolID]string),
	}
	ExtractModuleAliases(fc, inputs)
	if len(inputs.ModuleAliases) != 0 {
		t.Error("nil graph should produce no module aliases")
	}
}

func TestExtractModuleAliases_NilInputs(t *testing.T) {
	fc := &core.FlowContext{}
	ExtractModuleAliases(fc, nil)
}

func TestExtractDeclaredTypes_SoftAnnotationNotAnnotated(t *testing.T) {
	code := `local suites: {[string]: {any}} = {}`
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

	scopes := map[cfg.Point]*scope.State{
		graph.Entry(): scope.New(),
	}

	fc := &core.FlowContext{
		Graph:  graph,
		Scopes: scopes,
		Services: core.FlowServicesFuncs{
			TypeExprResolver: func(ast.TypeExpr, *scope.State) typ.Type {
				return typ.NewMap(typ.String, typ.NewArray(typ.Any))
			},
		},
	}
	inputs := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type),
	}
	ExtractDeclaredTypes(fc, inputs)

	var suitesSym cfg.SymbolID
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal {
			return
		}
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Name == "suites" {
				suitesSym = target.Symbol
			}
		}
	})
	if suitesSym == 0 {
		t.Fatal("failed to resolve suites symbol")
	}
	if inputs.AnnotatedVars != nil && inputs.AnnotatedVars[suitesSym] {
		t.Fatal("soft annotation should not mark AnnotatedVars")
	}
	if inputs.DeclaredTypes[suitesSym] == nil {
		t.Fatal("expected declared type for suites")
	}
}

func TestExtractDeclaredTypes_UnannotatedLocalFunctionStaysUnannotated(t *testing.T) {
	code := `
		local function a()
			return 1
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

	scopes := map[cfg.Point]*scope.State{
		graph.Entry(): scope.New(),
	}

	fc := &core.FlowContext{
		Graph:  graph,
		Scopes: scopes,
		Services: core.FlowServicesFuncs{
			// Simulate return-summary-enriched signature from upstream phases.
			FnSigResolver: func(*ast.FunctionExpr, *scope.State) *typ.Function {
				return typ.Func().Returns(typ.NewTypeParam("T", nil)).Build()
			},
		},
	}
	inputs := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type),
	}
	ExtractDeclaredTypes(fc, inputs)

	var symA cfg.SymbolID
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal {
			return
		}
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Name == "a" {
				symA = target.Symbol
			}
		}
	})
	if symA == 0 {
		t.Fatal("failed to resolve a symbol")
	}
	if inputs.AnnotatedVars != nil && inputs.AnnotatedVars[symA] {
		t.Fatal("unannotated local function should not be marked annotated")
	}
	if _, ok := inputs.DeclaredTypes[symA]; ok {
		t.Fatal("unannotated local function should not be added to DeclaredTypes")
	}
}

func TestExtractDeclaredTypes_ExplicitAnyMarksAnnotated(t *testing.T) {
	code := `local f: any = nil`
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

	scopes := map[cfg.Point]*scope.State{
		graph.Entry(): scope.New(),
	}

	fc := &core.FlowContext{
		Graph:  graph,
		Scopes: scopes,
		Services: core.FlowServicesFuncs{
			TypeExprResolver: func(ast.TypeExpr, *scope.State) typ.Type {
				return typ.Any
			},
		},
	}
	inputs := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type),
	}
	ExtractDeclaredTypes(fc, inputs)

	var symF cfg.SymbolID
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !info.IsLocal {
			return
		}
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Name == "f" {
				symF = target.Symbol
			}
		}
	})
	if symF == 0 {
		t.Fatal("failed to resolve f symbol")
	}
	if inputs.AnnotatedVars == nil || !inputs.AnnotatedVars[symF] {
		t.Fatal("explicit any annotation should mark AnnotatedVars")
	}
	if got := inputs.DeclaredTypes[symF]; got == nil || !typ.TypeEquals(got, typ.Any) {
		t.Fatalf("expected declared type any, got %v", got)
	}
}
