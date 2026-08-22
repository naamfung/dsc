package flowbuild_test

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/assign"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/decl"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/tblutil"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtract_NilGraph(t *testing.T) {
	result := flowbuild.Run(&core.FlowContext{})
	if result != nil {
		t.Error("expected nil for nil graph")
	}
}

func TestExtractFromConfig_NilGraph(t *testing.T) {
	result := flowbuild.Run(&core.FlowContext{})
	if result != nil {
		t.Error("expected nil for nil graph")
	}
}

func TestExtractFromConfig_MinimalGraph(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)

	result := flowbuild.Run(&core.FlowContext{
		Graph: graph,
		Base:  scope.New(),
	})
	if result == nil {
		t.Fatal("expected non-nil inputs")
	}
	if result.Graph != graph {
		t.Error("expected graph to be preserved")
	}
}

func TestExtractFromConfig_WithGlobals(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)

	globals := map[string]typ.Type{
		"print": &typ.Function{},
		"pairs": &typ.Function{},
	}

	result := flowbuild.Run(&core.FlowContext{
		Graph:   graph,
		Base:    scope.New(),
		Globals: globals,
	})
	if result == nil {
		t.Fatal("expected non-nil inputs")
	}
}

func TestExtractFromConfig_WithInitialDeclaredTypes(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)

	initial := flow.DeclaredTypes{
		cfg.SymbolID(1): typ.String,
		cfg.SymbolID(2): typ.Integer,
	}

	result := flowbuild.Run(&core.FlowContext{
		Graph:                graph,
		Base:                 scope.New(),
		InitialDeclaredTypes: initial,
	})
	if result == nil {
		t.Fatal("expected non-nil inputs")
	}
	if len(result.DeclaredTypes) < 2 {
		t.Error("expected initial declared types to be preserved")
	}
}

func TestExtractFromConfig_WithSiblingTypes(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)

	siblings := map[cfg.SymbolID]typ.Type{
		1: typ.String,
	}

	result := flowbuild.Run(&core.FlowContext{
		Graph:        graph,
		Base:         scope.New(),
		SiblingTypes: siblings,
	})
	if result == nil {
		t.Fatal("expected non-nil inputs")
	}
	if result.SiblingTypes == nil {
		t.Error("expected sibling types to be preserved")
	}
}

func TestExtractFromConfig_WithLiteralTypes(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)

	literals := map[cfg.SymbolID]typ.Type{
		1: &typ.Function{},
	}

	result := flowbuild.Run(&core.FlowContext{
		Graph:        graph,
		Base:         scope.New(),
		LiteralTypes: literals,
	})
	if result == nil {
		t.Fatal("expected non-nil inputs")
	}
	if result.LiteralTypes == nil {
		t.Error("expected literal types to be preserved")
	}
}

func TestBuildContextSymbolResolver_NilCtx(t *testing.T) {
	resolver := resolve.BuildContextSymbolResolver(nil)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	ty, ok := resolver(0, 1)
	if ok {
		t.Error("expected false for nil context")
	}
	if ty != nil {
		t.Error("expected nil type for nil context")
	}
}

func TestBuildContextTypeKeyResolver_NilCtx(t *testing.T) {
	resolver := resolve.BuildContextTypeKeyResolver(nil)
	if resolver == nil {
		t.Fatal("expected non-nil resolver")
	}
	// Test builtin type
	key, ok := resolver("number", nil)
	if !ok {
		t.Error("expected true for builtin type 'number'")
	}
	if key.IsZero() {
		t.Error("expected non-zero type key for 'number'")
	}
}

func TestBuildContextTypeKeyResolver_UnknownType(t *testing.T) {
	resolver := resolve.BuildContextTypeKeyResolver(nil)
	key, ok := resolver("NonExistentType", nil)
	if ok {
		t.Error("expected false for unknown type")
	}
	if !key.IsZero() {
		t.Error("expected zero type key for unknown type")
	}
}

func TestBuildRefinementLookup_NilCtx(t *testing.T) {
	symLookup := resolve.BuildRefinementLookup(nil)
	if symLookup != nil {
		t.Error("expected nil symLookup for nil context")
	}
}

func TestAddTypeKey_NilInputs(t *testing.T) {
	// Should not panic
	decl.AddTypeKey(nil, nil)
	decl.AddTypeKey(nil, typ.String)
	inputs := &flow.Inputs{TypeKeys: make(map[uint64]typ.Type)}
	decl.AddTypeKey(inputs, nil)
}

func TestAddTypeKey_BasicType(t *testing.T) {
	inputs := &flow.Inputs{TypeKeys: make(map[uint64]typ.Type)}
	decl.AddTypeKey(inputs, typ.String)
	if len(inputs.TypeKeys) == 0 {
		t.Error("expected type key to be added")
	}
}

func TestAddTypeKey_AliasType(t *testing.T) {
	inputs := &flow.Inputs{TypeKeys: make(map[uint64]typ.Type)}
	alias := &typ.Alias{Name: "MyString", Target: typ.String}
	decl.AddTypeKey(inputs, alias)
	// Should add both alias and target
	if len(inputs.TypeKeys) == 0 {
		t.Error("expected type keys to be added")
	}
}

func TestAddTypeKey_MetaType(t *testing.T) {
	inputs := &flow.Inputs{TypeKeys: make(map[uint64]typ.Type)}
	meta := &typ.Meta{Of: typ.String}
	decl.AddTypeKey(inputs, meta)
	if len(inputs.TypeKeys) == 0 {
		t.Error("expected type key to be added for meta type")
	}
}

func TestResolveRef_NilInputs(t *testing.T) {
	result := resolve.Ref(nil, nil)
	if result != nil {
		t.Error("expected nil for nil type")
	}
}

func TestResolveRef_NonRef(t *testing.T) {
	result := resolve.Ref(typ.String, nil)
	if result != typ.String {
		t.Error("expected same type for non-Ref")
	}
}

func TestResolveRef_UnresolvedRef(t *testing.T) {
	ref := &typ.Ref{Name: "MyType"}
	result := resolve.Ref(ref, nil)
	// Should return ref unchanged when scope is nil
	if result != ref {
		t.Error("expected same ref when scope is nil")
	}
}

func TestResolveRef_ResolvedRef(t *testing.T) {
	ref := &typ.Ref{Name: "MyType"}
	sc := scope.New().WithTypes(map[string]typ.Type{
		"MyType": typ.String,
	})
	result := resolve.Ref(ref, sc)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestFunctionHasAnnotations_NoAnnotations(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a", "b"},
		},
	}
	if tblutil.FunctionHasAnnotations(fn) {
		t.Error("expected false for function without annotations")
	}
}

func TestFunctionHasAnnotations_WithReturnTypes(t *testing.T) {
	fn := &ast.FunctionExpr{
		ReturnTypes: []ast.TypeExpr{
			&ast.PrimitiveTypeExpr{Name: "number"},
		},
	}
	if !tblutil.FunctionHasAnnotations(fn) {
		t.Error("expected true for function with return types")
	}
}

func TestFunctionHasAnnotations_WithParamTypes(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a"},
			Types: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "string"},
			},
		},
	}
	if !tblutil.FunctionHasAnnotations(fn) {
		t.Error("expected true for function with param types")
	}
}

func TestFunctionHasAnnotations_WithVarargType(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			HasVargs:   true,
			VarargType: &ast.PrimitiveTypeExpr{Name: "string"},
		},
	}
	if !tblutil.FunctionHasAnnotations(fn) {
		t.Error("expected true for function with vararg type")
	}
}

func TestFunctionHasAnnotations_NilFn(t *testing.T) {
	if tblutil.FunctionHasAnnotations(nil) {
		t.Error("expected false for nil function")
	}
}

func TestMergeCallConstraintsIntoEdges_Empty(t *testing.T) {
	inputs := &flow.Inputs{}
	flowbuild.MergeCallConstraintsIntoEdges(inputs, nil)
	if len(inputs.EdgeConditions) != 0 {
		t.Error("expected no edge conditions for empty input")
	}
}

func TestJoinTwo_LeftNil(t *testing.T) {
	result := typ.JoinPreferNonSoft(nil, typ.String)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestJoinTwo_RightNil(t *testing.T) {
	result := typ.JoinPreferNonSoft(typ.String, nil)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestJoinTwo_LeftUnknown(t *testing.T) {
	result := typ.JoinPreferNonSoft(typ.Unknown, typ.String)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestJoinTwo_RightUnknown(t *testing.T) {
	result := typ.JoinPreferNonSoft(typ.String, typ.Unknown)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestJoinTwo_Equal(t *testing.T) {
	result := typ.JoinPreferNonSoft(typ.String, typ.String)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestJoinTwo_Different(t *testing.T) {
	result := typ.JoinPreferNonSoft(typ.String, typ.Integer)
	union, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected Union, got %T", result)
	}
	if len(union.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(union.Members))
	}
}

func TestWidenArrayElementType_NilArray(t *testing.T) {
	result := flow.WidenArrayElementType(nil, typ.String, typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected Array, got %T", result)
	}
	if !typ.TypeEquals(arr.Element, typ.String) {
		t.Errorf("expected element String, got %v", arr.Element)
	}
}

func TestWidenArrayElementType_NilElement(t *testing.T) {
	arr := typ.NewArray(typ.String)
	result := flow.WidenArrayElementType(arr, nil, typ.JoinPreferNonSoft)
	if result != arr {
		t.Error("expected unchanged array for nil element")
	}
}

func TestWidenArrayElementType_Array(t *testing.T) {
	arr := typ.NewArray(typ.String)
	result := flow.WidenArrayElementType(arr, typ.Integer, typ.JoinPreferNonSoft)
	resultArr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected Array, got %T", result)
	}
	if resultArr.Element == nil {
		t.Error("expected non-nil element")
	}
}

func TestWidenArrayElementType_EmptyRecord(t *testing.T) {
	rec := typ.NewRecord().Build()
	result := flow.WidenArrayElementType(rec, typ.String, typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected Array, got %T", result)
	}
	if !typ.TypeEquals(arr.Element, typ.String) {
		t.Errorf("expected element String, got %v", arr.Element)
	}
}

func TestWidenArrayElementType_NonEmptyRecord(t *testing.T) {
	rec := typ.NewRecord().Field("a", typ.Integer).Build()
	result := flow.WidenArrayElementType(rec, typ.String, typ.JoinPreferNonSoft)
	if result != rec {
		t.Error("expected unchanged record")
	}
}

func TestWidenArrayElementType_Unknown(t *testing.T) {
	result := flow.WidenArrayElementType(typ.Unknown, typ.String, typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected Array, got %T", result)
	}
	if !typ.TypeEquals(arr.Element, typ.String) {
		t.Errorf("expected element String, got %v", arr.Element)
	}
}

func TestWidenArrayElementType_Any(t *testing.T) {
	result := flow.WidenArrayElementType(typ.Any, typ.String, typ.JoinPreferNonSoft)
	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("expected Array, got %T", result)
	}
	if !typ.TypeEquals(arr.Element, typ.String) {
		t.Errorf("expected element String, got %v", arr.Element)
	}
}

func TestMergeSpecTypes_Empty(t *testing.T) {
	result := assign.MergeSpecTypes(nil, nil)
	if result != nil {
		t.Error("expected nil for empty inputs")
	}
}

func TestMergeSpecTypes_BaseOnly(t *testing.T) {
	base := api.SpecTypes{1: typ.String}
	result := assign.MergeSpecTypes(base, nil)
	if result[1] != typ.String {
		t.Error("expected base types to be preserved")
	}
}

func TestMergeSpecTypes_OverrideOnly(t *testing.T) {
	override := api.SpecTypes{1: typ.Integer}
	result := assign.MergeSpecTypes(nil, override)
	if result[1] != typ.Integer {
		t.Error("expected override types to be used")
	}
}

func TestMergeSpecTypes_OverrideTakesPrecedence(t *testing.T) {
	base := api.SpecTypes{1: typ.String}
	override := api.SpecTypes{1: typ.Integer}
	result := assign.MergeSpecTypes(base, override)
	if result[1] != typ.Integer {
		t.Error("expected override to take precedence")
	}
}

func TestIsUnknownOrNil(t *testing.T) {
	tests := []struct {
		name     string
		t        typ.Type
		expected bool
	}{
		{"nil", nil, true},
		{"unknown", typ.Unknown, true},
		{"nil_type", typ.Nil, true},
		{"string", typ.String, false},
		{"integer", typ.Integer, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := typ.IsUnknownOrNil(tt.t)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSortPoints(t *testing.T) {
	points := []cfg.Point{5, 3, 1, 4, 2}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	for i := 0; i < len(points)-1; i++ {
		if points[i] > points[i+1] {
			t.Errorf("points not sorted at index %d: %v", i, points)
			break
		}
	}
}

func TestSortPoints_Empty(t *testing.T) {
	var points []cfg.Point
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] }) // Should not panic
}

func TestSortPoints_Single(t *testing.T) {
	points := []cfg.Point{1}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] }) // Should not panic
	if len(points) != 1 || points[0] != 1 {
		t.Error("single element should remain unchanged")
	}
}

func TestExtractTypeKeys_NilScope(t *testing.T) {
	inputs := &flow.Inputs{TypeKeys: make(map[uint64]typ.Type)}
	decl.ExtractTypeKeys(&core.FlowContext{Base: nil}, inputs)
	if len(inputs.TypeKeys) != 0 {
		t.Error("expected no type keys for nil scope")
	}
}

func TestExtractTypeKeys_WithTypes(t *testing.T) {
	inputs := &flow.Inputs{TypeKeys: make(map[uint64]typ.Type)}
	sc := scope.New().WithTypes(map[string]typ.Type{
		"MyType": typ.String,
	})
	decl.ExtractTypeKeys(&core.FlowContext{Base: sc}, inputs)
	if len(inputs.TypeKeys) == 0 {
		t.Error("expected type keys to be extracted")
	}
}

func TestTableHasFunctionField_Empty(t *testing.T) {
	tbl := &ast.TableExpr{}
	if tblutil.TableHasFunctionField(tbl) {
		t.Error("expected false for empty table")
	}
}

func TestTableHasFunctionField_NoFunction(t *testing.T) {
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "a"}, Value: &ast.NumberExpr{Value: "1"}},
		},
	}
	if tblutil.TableHasFunctionField(tbl) {
		t.Error("expected false for table without function fields")
	}
}

func TestTableHasFunctionField_WithFunction(t *testing.T) {
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "fn"}, Value: &ast.FunctionExpr{}},
		},
	}
	if !tblutil.TableHasFunctionField(tbl) {
		t.Error("expected true for table with function field")
	}
}

func TestTableHasFunctionField_Nil(t *testing.T) {
	if tblutil.TableHasFunctionField(nil) {
		t.Error("expected false for nil table")
	}
}

func TestSynthTableLiteralWithWrapper_Nil(t *testing.T) {
	result := tblutil.SynthTableLiteralWithWrapper(nil, 0, nil)
	if result != nil {
		t.Error("expected nil for nil table")
	}
}

func TestSynthTableLiteralWithWrapper_Empty(t *testing.T) {
	tbl := &ast.TableExpr{}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Unknown
	}
	result := tblutil.SynthTableLiteralWithWrapper(tbl, 0, synth)
	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}
	if len(rec.Fields) != 0 {
		t.Error("expected empty record")
	}
}

func TestSynthTableLiteralWithWrapper_WithFields(t *testing.T) {
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "name"}, Value: &ast.StringExpr{Value: "test"}},
		},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	result := tblutil.SynthTableLiteralWithWrapper(tbl, 0, synth)
	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", result)
	}
	if len(rec.Fields) != 1 {
		t.Errorf("expected 1 field, got %d", len(rec.Fields))
	}
}

func TestSynthTableLiteralWithWrapper_ArrayElements(t *testing.T) {
	tbl := &ast.TableExpr{
		Fields: []*ast.Field{
			{Value: &ast.NumberExpr{Value: "1"}},
			{Value: &ast.NumberExpr{Value: "2"}},
		},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Integer
	}
	result := tblutil.SynthTableLiteralWithWrapper(tbl, 0, synth)
	tuple, ok := result.(*typ.Tuple)
	if !ok {
		t.Fatalf("expected Tuple, got %T", result)
	}
	if len(tuple.Elements) != 2 {
		t.Errorf("expected 2 elements, got %d", len(tuple.Elements))
	}
}

func TestBuildContextTypeKeyResolver_BuiltinTypes(t *testing.T) {
	resolver := resolve.BuildContextTypeKeyResolver(nil)

	builtins := []string{"number", "string", "boolean", "nil", "function", "table", "thread", "userdata"}
	for _, name := range builtins {
		if kind.FromString(name) == kind.Unknown {
			continue // Skip if not a recognized builtin
		}
		key, ok := resolver(name, nil)
		if !ok {
			t.Errorf("expected true for builtin type '%s'", name)
		}
		if key.IsZero() {
			t.Errorf("expected non-zero key for builtin type '%s'", name)
		}
	}
}
