package resolve

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/scope"
	typecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func newTestResolver() *Resolver {
	return New(Config{})
}

type testParamBindings struct {
	params map[*ast.FunctionExpr][]typecfg.SymbolID
	names  map[typecfg.SymbolID]string
}

type manifestQuerierStub struct {
	manifests map[string]*io.Manifest
	imports   map[string]*io.Manifest
}

func (m manifestQuerierStub) Manifest(path string) *io.Manifest {
	return m.manifests[path]
}

func (m manifestQuerierStub) Imports() map[string]*io.Manifest {
	return m.imports
}

func (m testParamBindings) ParamSymbols(fn *ast.FunctionExpr) []typecfg.SymbolID {
	return m.params[fn]
}

func (m testParamBindings) Name(sym typecfg.SymbolID) string {
	return m.names[sym]
}

func TestResolveType_Nil(t *testing.T) {
	r := newTestResolver()
	result := r.ResolveType(nil, nil)
	if result != typ.Unknown {
		t.Fatalf("got %v, want unknown", result)
	}
}

func TestResolveType_Primitives(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	tests := []struct {
		name     string
		expected typ.Type
	}{
		{"nil", typ.Nil},
		{"boolean", typ.Boolean},
		{"number", typ.Number},
		{"integer", typ.Integer},
		{"string", typ.String},
		{"any", typ.Any},
		{"unknown", typ.Unknown},
		{"never", typ.Never},
	}

	for _, tt := range tests {
		result := r.ResolveType(&ast.PrimitiveTypeExpr{Name: tt.name}, sc)
		if result != tt.expected {
			t.Errorf("%s: got %v, want %v", tt.name, result, tt.expected)
		}
	}
}

func TestResolveType_Self(t *testing.T) {
	r := newTestResolver()
	selfType := typ.NewRecord().Field("id", typ.Integer).Build()
	sc := scope.New().WithSelf(selfType)

	result := r.ResolveType(&ast.PrimitiveTypeExpr{Name: "Self"}, sc)
	if result != selfType {
		t.Fatalf("got %v, want self type", result)
	}
}

func TestResolveType_SelfNoScope(t *testing.T) {
	r := newTestResolver()
	result := r.ResolveType(&ast.SelfTypeExpr{}, nil)
	if result != typ.Self {
		t.Fatalf("got %v, want Self", result)
	}
}

func TestResolveType_ModuleAliasPrefixUsesRequireAliasPath(t *testing.T) {
	manifest := io.NewManifest("store")
	storeType := typ.NewAlias("Store", typ.NewRecord().
		Field("cache", typ.NewMap(typ.String, typ.String)).
		Build())
	manifest.DefineType("Store", storeType)

	bt := bind.NewBindingTable()
	const sym typecfg.SymbolID = 42
	bt.SetName(sym, "store_mod")

	r := New(Config{
		Manifests: manifestQuerierStub{
			manifests: map[string]*io.Manifest{"store": manifest},
		},
		ModuleBindings: bt,
		ModuleAliases:  map[typecfg.SymbolID]string{sym: "store"},
	})

	result := r.ResolveType(&ast.TypeRefExpr{Path: []string{"store_mod", "Store"}}, scope.New())
	if !typ.TypeEquals(result, storeType) {
		t.Fatalf("got %s, want %s", typ.FormatShort(result), typ.FormatShort(storeType))
	}
}

func TestResolveType_Optional(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.OptionalTypeExpr{Inner: &ast.PrimitiveTypeExpr{Name: "string"}}
	result := r.ResolveType(expr, sc)

	opt, ok := result.(*typ.Optional)
	if !ok {
		t.Fatalf("got %T, want optional", result)
	}
	if opt.Inner != typ.String {
		t.Fatalf("inner: got %v, want string", opt.Inner)
	}
}

func TestResolveType_Union(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.UnionTypeExpr{
		Types: []ast.TypeExpr{
			&ast.PrimitiveTypeExpr{Name: "string"},
			&ast.PrimitiveTypeExpr{Name: "number"},
		},
	}
	result := r.ResolveType(expr, sc)

	union, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("got %T, want union", result)
	}
	if len(union.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(union.Members))
	}
}

func TestResolveType_UnionEmpty(t *testing.T) {
	r := newTestResolver()
	result := r.ResolveType(&ast.UnionTypeExpr{Types: nil}, nil)
	if result != typ.Never {
		t.Fatalf("got %v, want never", result)
	}
}

func TestResolveType_Intersection(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.IntersectionTypeExpr{
		Types: []ast.TypeExpr{
			&ast.PrimitiveTypeExpr{Name: "string"},
			&ast.PrimitiveTypeExpr{Name: "number"},
		},
	}
	result := r.ResolveType(expr, sc)

	inter, ok := result.(*typ.Intersection)
	if !ok {
		t.Fatalf("got %T, want intersection", result)
	}
	if len(inter.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(inter.Members))
	}
}

func TestResolveType_IntersectionEmpty(t *testing.T) {
	r := newTestResolver()
	result := r.ResolveType(&ast.IntersectionTypeExpr{Types: nil}, nil)
	if result != typ.Any {
		t.Fatalf("got %v, want any", result)
	}
}

func TestResolveType_Array(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.ArrayTypeExpr{Element: &ast.PrimitiveTypeExpr{Name: "integer"}}
	result := r.ResolveType(expr, sc)

	arr, ok := result.(*typ.Array)
	if !ok {
		t.Fatalf("got %T, want array", result)
	}
	if arr.Element != typ.Integer {
		t.Fatalf("element: got %v, want integer", arr.Element)
	}
}

func TestResolveType_Map(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.MapTypeExpr{
		Key:   &ast.PrimitiveTypeExpr{Name: "string"},
		Value: &ast.PrimitiveTypeExpr{Name: "number"},
	}
	result := r.ResolveType(expr, sc)

	m, ok := result.(*typ.Map)
	if !ok {
		t.Fatalf("got %T, want map", result)
	}
	if m.Key != typ.String {
		t.Fatalf("key: got %v, want string", m.Key)
	}
	if m.Value != typ.Number {
		t.Fatalf("value: got %v, want number", m.Value)
	}
}

func TestResolveType_Record(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.RecordTypeExpr{
		Fields: []ast.RecordFieldExpr{
			{Name: "x", Type: &ast.PrimitiveTypeExpr{Name: "number"}},
			{Name: "y", Type: &ast.PrimitiveTypeExpr{Name: "number"}, Optional: true},
		},
	}
	result := r.ResolveType(expr, sc)

	rec, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result)
	}

	xField := rec.GetField("x")
	if xField == nil {
		t.Fatal("missing x field")
	}
	if xField.Optional {
		t.Fatal("x should not be optional")
	}

	yField := rec.GetField("y")
	if yField == nil {
		t.Fatal("missing y field")
	}
	if !yField.Optional {
		t.Fatal("y should be optional")
	}
}

func TestResolveType_Function(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.FunctionTypeExpr{
		Params: []ast.FunctionParamExpr{
			{Name: "a", Type: &ast.PrimitiveTypeExpr{Name: "string"}},
		},
		Returns: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
	}
	result := r.ResolveType(expr, sc)

	fn, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("got %T, want function", result)
	}
	if len(fn.Params) != 1 {
		t.Fatalf("got %d params, want 1", len(fn.Params))
	}
	if len(fn.Returns) != 1 {
		t.Fatalf("got %d returns, want 1", len(fn.Returns))
	}
}

func TestResolveType_FunctionOptionalParam(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.FunctionTypeExpr{
		Params: []ast.FunctionParamExpr{
			{Name: "n", Type: &ast.OptionalTypeExpr{Inner: &ast.PrimitiveTypeExpr{Name: "number"}}},
		},
		Returns: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "string"}},
	}
	result := r.ResolveType(expr, sc)

	fn, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("got %T, want function", result)
	}
	if len(fn.Params) != 1 {
		t.Fatalf("got %d params, want 1", len(fn.Params))
	}
	if !fn.Params[0].Optional {
		t.Fatal("expected function type optional param to preserve optional arity")
	}
	if !typ.TypeEquals(fn.Params[0].Type, typ.NewOptional(typ.Number)) {
		t.Fatalf("got param type %v, want number?", fn.Params[0].Type)
	}
}

func TestResolveType_FunctionVariadic(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.FunctionTypeExpr{
		Variadic: &ast.PrimitiveTypeExpr{Name: "any"},
	}
	result := r.ResolveType(expr, sc)

	fn, ok := result.(*typ.Function)
	if !ok {
		t.Fatalf("got %T, want function", result)
	}
	if fn.Variadic != typ.Any {
		t.Fatalf("variadic: got %v, want any", fn.Variadic)
	}
}

func TestResolveType_TypeRef_Simple(t *testing.T) {
	r := newTestResolver()
	myType := typ.NewRecord().Field("id", typ.Integer).Build()
	sc := scope.New().WithType("MyType", myType)

	expr := &ast.TypeRefExpr{Path: []string{"MyType"}}
	result := r.ResolveType(expr, sc)

	if result != myType {
		t.Fatalf("got %v, want MyType", result)
	}
}

func TestResolveType_TypeRef_Unknown(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.TypeRefExpr{Path: []string{"UnknownType"}}
	result := r.ResolveType(expr, sc)

	ref, ok := result.(*typ.Ref)
	if !ok {
		t.Fatalf("got %T, want ref", result)
	}
	if ref.Name != "UnknownType" {
		t.Fatalf("got %q, want %q", ref.Name, "UnknownType")
	}
}

func TestResolveType_TypeRef_Module(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.TypeRefExpr{Path: []string{"mymodule", "MyType"}}
	result := r.ResolveType(expr, sc)

	ref, ok := result.(*typ.Ref)
	if !ok {
		t.Fatalf("got %T, want ref", result)
	}
	if ref.Module != "mymodule" {
		t.Fatalf("module: got %q, want %q", ref.Module, "mymodule")
	}
	if ref.Name != "MyType" {
		t.Fatalf("name: got %q, want %q", ref.Name, "MyType")
	}
}

func TestResolveType_TypeRef_ModuleImportsFallback(t *testing.T) {
	manifest := io.NewManifest("mymodule")
	manifest.DefineType("MyType", typ.Integer)
	r := New(Config{
		Manifests: manifestQuerierStub{
			manifests: map[string]*io.Manifest{},
			imports:   map[string]*io.Manifest{"mymodule": manifest},
		},
	})
	sc := scope.New()

	expr := &ast.TypeRefExpr{Path: []string{"mymodule", "MyType"}}
	result := r.ResolveType(expr, sc)
	if !typ.TypeEquals(result, typ.Integer) {
		t.Fatalf("got %v, want integer from imports fallback", result)
	}
}

func TestResolveType_Literal_String(t *testing.T) {
	r := newTestResolver()
	result := r.ResolveType(&ast.LiteralTypeExpr{Value: "hello"}, nil)

	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Base != kind.String {
		t.Fatalf("base: got %v, want string", lit.Base)
	}
	if lit.Value != "hello" {
		t.Fatalf("value: got %v, want %q", lit.Value, "hello")
	}
}

func TestResolveType_Literal_Number(t *testing.T) {
	r := newTestResolver()
	result := r.ResolveType(&ast.LiteralTypeExpr{Value: 3.14}, nil)

	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Base != kind.Number {
		t.Fatalf("base: got %v, want number", lit.Base)
	}
}

func TestResolveType_Literal_Int(t *testing.T) {
	r := newTestResolver()
	result := r.ResolveType(&ast.LiteralTypeExpr{Value: int64(42)}, nil)

	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Base != kind.Integer {
		t.Fatalf("base: got %v, want integer", lit.Base)
	}
}

func TestResolveType_Literal_Bool(t *testing.T) {
	r := newTestResolver()
	result := r.ResolveType(&ast.LiteralTypeExpr{Value: true}, nil)

	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Base != kind.Boolean {
		t.Fatalf("base: got %v, want boolean", lit.Base)
	}
}

func TestResolveType_Tuple(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.TupleTypeExpr{
		Elements: []ast.TypeExpr{
			&ast.PrimitiveTypeExpr{Name: "string"},
			&ast.PrimitiveTypeExpr{Name: "number"},
			&ast.PrimitiveTypeExpr{Name: "boolean"},
		},
	}
	result := r.ResolveType(expr, sc)

	tuple, ok := result.(*typ.Tuple)
	if !ok {
		t.Fatalf("got %T, want tuple", result)
	}
	if len(tuple.Elements) != 3 {
		t.Fatalf("got %d elements, want 3", len(tuple.Elements))
	}
}

func TestResolveType_Meta(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	expr := &ast.MetaTypeExpr{Inner: &ast.PrimitiveTypeExpr{Name: "string"}}
	result := r.ResolveType(expr, sc)

	meta, ok := result.(*typ.Meta)
	if !ok {
		t.Fatalf("got %T, want meta", result)
	}
	if meta.Of != typ.String {
		t.Fatalf("inner: got %v, want string", meta.Of)
	}
}

func TestComputeKeyOf_Record(t *testing.T) {
	rec := typ.NewRecord().
		Field("a", typ.Integer).
		Field("b", typ.String).
		Build()

	result := ComputeKeyOf(rec)

	union, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("got %T, want union", result)
	}
	if len(union.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(union.Members))
	}
}

func TestComputeKeyOf_EmptyRecord(t *testing.T) {
	rec := typ.NewRecord().Build()
	result := ComputeKeyOf(rec)
	if result != typ.Never {
		t.Fatalf("got %v, want never", result)
	}
}

func TestComputeKeyOf_Map(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Integer)
	result := ComputeKeyOf(m)
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestComputeKeyOf_Array(t *testing.T) {
	arr := typ.NewArray(typ.String)
	result := ComputeKeyOf(arr)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestComputeIndexAccess_Record(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	key := typ.LiteralString("name")

	result := ComputeIndexAccess(rec, key)
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestComputeIndexAccess_Map(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Integer)
	key := typ.LiteralString("any")

	result := ComputeIndexAccess(m, key)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestComputeIndexAccess_Array(t *testing.T) {
	arr := typ.NewArray(typ.Boolean)
	key := typ.LiteralInt(1)

	result := ComputeIndexAccess(arr, key)
	if result != typ.Boolean {
		t.Fatalf("got %v, want boolean", result)
	}
}

func TestComputeIndexAccess_Tuple(t *testing.T) {
	tuple := typ.NewTuple(typ.String, typ.Integer, typ.Boolean)
	key := typ.LiteralInt(2)

	result := ComputeIndexAccess(tuple, key)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer (second element)", result)
	}
}

func TestComputeConditionalType_True(t *testing.T) {
	check := typ.String
	extends := typ.String
	thenFn := func() typ.Type { return typ.Integer }
	elseFn := func() typ.Type { return typ.Boolean }

	result := ComputeConditionalType(check, extends, thenFn, elseFn)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer (then branch)", result)
	}
}

func TestComputeConditionalType_False(t *testing.T) {
	check := typ.String
	extends := typ.Number
	thenFn := func() typ.Type { return typ.Integer }
	elseFn := func() typ.Type { return typ.Boolean }

	result := ComputeConditionalType(check, extends, thenFn, elseFn)
	if result != typ.Boolean {
		t.Fatalf("got %v, want boolean (else branch)", result)
	}
}

func TestComputeConditionalType_DistributeUnion(t *testing.T) {
	check := typ.NewUnion(typ.String, typ.Number)
	extends := typ.String
	thenFn := func() typ.Type { return typ.True }
	elseFn := func() typ.Type { return typ.False }

	result := ComputeConditionalType(check, extends, thenFn, elseFn)

	union, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("got %T, want union", result)
	}
	if len(union.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(union.Members))
	}
}

func TestResolveType_DepthLimit(t *testing.T) {
	r := newTestResolver()

	var buildDeep func(depth int) ast.TypeExpr
	buildDeep = func(depth int) ast.TypeExpr {
		if depth > 100 {
			return &ast.PrimitiveTypeExpr{Name: "string"}
		}
		return &ast.OptionalTypeExpr{Inner: buildDeep(depth + 1)}
	}

	expr := buildDeep(0)
	result := r.ResolveType(expr, nil)

	if result == nil {
		t.Fatal("should handle deep nesting")
	}
}

func TestResolveNamed_TypeParam(t *testing.T) {
	r := newTestResolver()
	typeParam := typ.NewTypeParam("T", nil)
	sc := scope.New().WithTypeParams(map[string]typ.Type{"T": typeParam})

	result := r.ResolveType(&ast.PrimitiveTypeExpr{Name: "T"}, sc)
	if result != typeParam {
		t.Fatalf("got %v, want type param T", result)
	}
}

func TestResolveNamed_Type(t *testing.T) {
	r := newTestResolver()
	myType := typ.NewRecord().Build()
	sc := scope.New().WithType("MyRecord", myType)

	result := r.ResolveType(&ast.PrimitiveTypeExpr{Name: "MyRecord"}, sc)
	if result != myType {
		t.Fatalf("got %v, want MyRecord", result)
	}
}

func TestResolveNamed_Unknown(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	result := r.ResolveType(&ast.PrimitiveTypeExpr{Name: "Unknown"}, sc)

	ref, ok := result.(*typ.Ref)
	if !ok {
		t.Fatalf("got %T, want ref", result)
	}
	if ref.Name != "Unknown" {
		t.Fatalf("got %q, want %q", ref.Name, "Unknown")
	}
}

func TestResolveFunctionSignature(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a", "b"},
			Types: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "string"},
				&ast.PrimitiveTypeExpr{Name: "number"},
			},
		},
		ReturnTypes: []ast.TypeExpr{
			&ast.PrimitiveTypeExpr{Name: "boolean"},
		},
	}

	result := r.ResolveFunctionSignature(fn, sc)
	if result == nil {
		t.Fatal("expected function type")
	}
	if len(result.Params) != 2 {
		t.Fatalf("got %d params, want 2", len(result.Params))
	}
	if len(result.Returns) != 1 {
		t.Fatalf("got %d returns, want 1", len(result.Returns))
	}
}

func TestResolveFunctionSignature_ImplicitSelfFromBindings(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
		},
	}
	r := New(Config{
		Bindings: testParamBindings{
			params: map[*ast.FunctionExpr][]typecfg.SymbolID{
				fn: {1, 2},
			},
			names: map[typecfg.SymbolID]string{
				1: "self",
				2: "x",
			},
		},
	})
	sc := scope.New().WithSelf(typ.String)

	result := r.ResolveFunctionSignature(fn, sc)
	if result == nil {
		t.Fatal("expected function type")
	}
	if len(result.Params) != 2 {
		t.Fatalf("got %d params, want 2 (self + x)", len(result.Params))
	}
	if result.Params[0].Name != "self" || result.Params[0].Type != typ.String {
		t.Fatalf("unexpected self param: %+v", result.Params[0])
	}
}

func TestResolveReturnTypes_Tuple(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	types := []ast.TypeExpr{
		&ast.TupleTypeExpr{
			Elements: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "string"},
				&ast.PrimitiveTypeExpr{Name: "number"},
			},
		},
	}

	result := r.ResolveReturnTypes(types, sc)
	if len(result) != 2 {
		t.Fatalf("got %d types, want 2 (tuple expanded)", len(result))
	}
}

func TestResolveTypeDef_Simple(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	result := r.ResolveTypeDef("MyInt", &ast.PrimitiveTypeExpr{Name: "integer"}, nil, sc)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestResolveTypeDef_Generic(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	typeParams := []ast.TypeParamExpr{
		{Name: "T"},
	}
	typeExpr := &ast.ArrayTypeExpr{
		Element: &ast.PrimitiveTypeExpr{Name: "T"},
	}

	result := r.ResolveTypeDef("List", typeExpr, typeParams, sc)
	generic, ok := result.(*typ.Generic)
	if !ok {
		t.Fatalf("got %T, want generic", result)
	}
	if generic.Name != "List" {
		t.Fatalf("got %q, want List", generic.Name)
	}
	if len(generic.TypeParams) != 1 {
		t.Fatalf("got %d type params, want 1", len(generic.TypeParams))
	}
}

func TestResolveTypeDef_RecursiveAliasBodyUsesResolvedSelfType(t *testing.T) {
	r := newTestResolver()
	sc := scope.New()

	typeExpr := &ast.RecordTypeExpr{
		Fields: []ast.RecordFieldExpr{
			{
				Name: "f",
				Type: &ast.FunctionTypeExpr{
					Params: []ast.FunctionParamExpr{
						{Name: "self", Type: &ast.PrimitiveTypeExpr{Name: "Node"}},
					},
					Returns: []ast.TypeExpr{
						&ast.PrimitiveTypeExpr{Name: "Node"},
					},
				},
			},
			{
				Name: "g",
				Type: &ast.FunctionTypeExpr{
					Params: []ast.FunctionParamExpr{
						{Name: "self", Type: &ast.PrimitiveTypeExpr{Name: "Node"}},
					},
					Returns: []ast.TypeExpr{
						&ast.PrimitiveTypeExpr{Name: "number"},
					},
				},
			},
		},
	}

	result := r.ResolveTypeDef("Node", typeExpr, nil, sc)
	rec, ok := result.(*typ.Recursive)
	if !ok {
		t.Fatalf("got %T, want recursive type", result)
	}

	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("body: got %T, want record", rec.Body)
	}

	fField := body.GetField("f")
	if fField == nil {
		t.Fatal("missing f field")
	}
	fType, ok := fField.Type.(*typ.Function)
	if !ok {
		t.Fatalf("f: got %T, want function", fField.Type)
	}
	if len(fType.Params) != 1 || fType.Params[0].Type != rec {
		t.Fatalf("f self param: got %v, want recursive self type", fType.Params[0].Type)
	}
	if len(fType.Returns) != 1 || fType.Returns[0] != rec {
		t.Fatalf("f return: got %v, want recursive self type", fType.Returns[0])
	}
}
