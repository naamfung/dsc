package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/extract"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

type mockManifestQuerier struct {
	manifests map[string]*io.Manifest
	imports   map[string]*io.Manifest
}

func (m mockManifestQuerier) Manifest(path string) *io.Manifest {
	if m.manifests == nil {
		return nil
	}
	return m.manifests[path]
}

func (m mockManifestQuerier) Imports() map[string]*io.Manifest {
	if m.imports != nil {
		return m.imports
	}
	return m.manifests
}

func TestSynthCallCore_SimpleCall(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	const sym = cfg.SymbolID(1)
	graph := mockGraph{symbols: map[string]cfg.SymbolID{"greet": sym}}
	declared := flow.DeclaredTypes{sym: fn}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		DeclaredTypes: declared,
	})

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Env:    checkCtx,
	})

	expr := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "greet"},
		Args: []ast.Expr{},
	}

	result := e.MultiTypeOf(expr, 0)
	if len(result) == 0 {
		t.Fatal("expected at least one return type")
	}
}

func TestSynthCallCore_DeclaredSpecReturnNarrowing(t *testing.T) {
	spec := contract.NewSpec().
		WithReturnCase(
			constraint.FromConstraints(constraint.FieldEquals{
				Target: constraint.Path{Root: "$1"},
				Field:  "message",
				Value:  typ.LiteralBool(true),
			}),
			typ.String,
		).
		WithDefaultReturn(typ.Any)

	fn := typ.Func().
		Param("topic", typ.String).
		OptParam("opts", typ.NewRecord().OptField("message", typ.Boolean).Build()).
		Returns(typ.Any).
		Spec(spec).
		Build()

	const sym = cfg.SymbolID(1)
	listenIdent := &ast.IdentExpr{Value: "listen"}

	bindings := bind.NewBindingTable()
	bindings.Bind(listenIdent, sym)

	graph := mockGraph{symbols: map[string]cfg.SymbolID{"listen": sym}}
	declared := flow.DeclaredTypes{sym: fn}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		Bindings:      bindings,
		DeclaredTypes: declared,
	})

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Env:    checkCtx,
		Phase:  api.PhaseScopeCompute,
	})

	expr := &ast.FuncCallExpr{
		Func: listenIdent,
		Args: []ast.Expr{
			&ast.StringExpr{Value: "topic"},
			&ast.TableExpr{
				Fields: []*ast.Field{
					{Key: &ast.IdentExpr{Value: "message"}, Value: &ast.TrueExpr{}},
				},
			},
		},
	}

	result := e.MultiTypeOf(expr, 0)
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	if result[0] != typ.String {
		t.Fatalf("got %v, want string", result[0])
	}
}

func TestSynthCallCore_TypeCast(t *testing.T) {
	meta := typ.NewMeta(typ.Integer)
	sc := scope.New().WithType("MyInt", meta)
	scopes := make(api.ScopeMap)
	scopes[cfg.Point(0)] = sc

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: scopes,
	})

	expr := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "MyInt"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}

	result := e.MultiTypeOf(expr, 0)
	if len(result) == 0 {
		t.Fatal("expected at least one result")
	}
}

func TestSynthCallCore_Require(t *testing.T) {
	exportType := typ.NewRecord().Field("version", typ.String).Build()
	manifest := io.NewManifest("mymodule")
	manifest.Export = exportType

	manifests := mockManifestQuerier{
		manifests: map[string]*io.Manifest{"mymodule": manifest},
	}

	sc := scope.New()
	scopes := make(api.ScopeMap)
	scopes[cfg.Point(0)] = sc

	requireFn := typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Effects(effect.WithModuleLoad()).
		Build()

	graph := mockGraph{symbols: map[string]cfg.SymbolID{}}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:       graph,
		GlobalTypes: map[string]typ.Type{"require": requireFn},
	})

	e := New(Config{
		Ctx:       db.NewQueryContext(db.New()),
		Types:     mockTypeQuerier{},
		Scopes:    scopes,
		Manifests: manifests,
		Env:       checkCtx,
	})

	expr := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "mymodule"}},
	}

	result := e.MultiTypeOf(expr, 0)
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}

	rec, ok := result[0].(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result[0])
	}
	if rec.GetField("version") == nil {
		t.Fatal("expected version field")
	}
}

func TestSynthCallCore_RequireUnknownModule(t *testing.T) {
	manifests := mockManifestQuerier{manifests: map[string]*io.Manifest{}}

	sc := scope.New()
	scopes := make(api.ScopeMap)
	scopes[cfg.Point(0)] = sc

	e := New(Config{
		Ctx:       db.NewQueryContext(db.New()),
		Types:     mockTypeQuerier{},
		Scopes:    scopes,
		Manifests: manifests,
	})

	expr := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "unknown"}},
	}

	result := e.MultiTypeOf(expr, 0)
	if len(result) == 0 {
		t.Fatal("expected at least one result")
	}
}

func TestSynthCallCore_RequireImportAlias(t *testing.T) {
	exportType := typ.NewRecord().Field("is_nil", typ.Func().Returns(typ.Boolean).Build()).Build()
	manifest := io.NewManifest("actual_module")
	manifest.Export = exportType

	manifests := mockManifestQuerier{
		manifests: map[string]*io.Manifest{"actual_module": manifest},
		imports:   map[string]*io.Manifest{"assert_primitives": manifest},
	}

	sc := scope.New()
	scopes := make(api.ScopeMap)
	scopes[cfg.Point(0)] = sc

	requireFn := typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Effects(effect.WithModuleLoad()).
		Build()

	graph := mockGraph{symbols: map[string]cfg.SymbolID{}}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:       graph,
		GlobalTypes: map[string]typ.Type{"require": requireFn},
	})

	e := New(Config{
		Ctx:       db.NewQueryContext(db.New()),
		Types:     mockTypeQuerier{},
		Scopes:    scopes,
		Manifests: manifests,
		Env:       checkCtx,
	})

	expr := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "require"},
		Args: []ast.Expr{&ast.StringExpr{Value: "assert_primitives"}},
	}

	result := e.MultiTypeOf(expr, 0)
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}

	if result[0] == nil || result[0] == typ.Nil {
		t.Fatal("require with import alias should resolve to manifest export, got nil")
	}

	rec, ok := result[0].(*typ.Record)
	if !ok {
		t.Fatalf("got %T, want record", result[0])
	}
	if rec.GetField("is_nil") == nil {
		t.Fatal("expected is_nil field from manifest export")
	}
}

func TestSynthMethodCallCore(t *testing.T) {
	methodFn := typ.Func().Param("self", typ.Any).Returns(typ.String).Build()
	rec := typ.NewRecord().Field("getName", methodFn).Build()
	const sym = cfg.SymbolID(1)
	graph := mockGraph{symbols: map[string]cfg.SymbolID{"obj": sym}}
	declared := flow.DeclaredTypes{sym: rec}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		DeclaredTypes: declared,
	})

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Env:    checkCtx,
	})

	expr := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "obj"},
		Method:   "getName",
		Args:     []ast.Expr{},
	}

	result := e.MultiTypeOf(expr, 0)
	if len(result) == 0 {
		t.Fatal("expected at least one result")
	}
}

func TestEngineCallQuery_Field(t *testing.T) {
	e := newTestEngine()
	q := e.CallQuery()

	rec := typ.NewRecord().Field("x", typ.Integer).Build()
	result, ok := q.Field(nil, rec, "x")
	if !ok {
		t.Fatal("expected field lookup to succeed")
	}
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestEngineCallQuery_FieldMissing(t *testing.T) {
	e := newTestEngine()
	q := e.CallQuery()

	rec := typ.NewRecord().Field("x", typ.Integer).Build()
	_, ok := q.Field(nil, rec, "y")
	if ok {
		t.Fatal("expected field lookup to fail")
	}
}

func TestEngineCallQuery_Method(t *testing.T) {
	e := newTestEngine()
	q := e.CallQuery()

	_, ok := q.Method(nil, typ.String, "upper")
	if ok {
		t.Fatal("mock doesn't implement methods")
	}
}

func TestEngineCallQuery_NilTypes(t *testing.T) {
	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  nil,
		Scopes: make(api.ScopeMap),
	})
	q := e.CallQuery()

	_, ok := q.Field(nil, typ.String, "x")
	if ok {
		t.Fatal("nil types should return false")
	}

	_, ok = q.Method(nil, typ.String, "x")
	if ok {
		t.Fatal("nil types should return false")
	}
}

func TestCopyTypes_Empty(t *testing.T) {
	result := extract.CopyTypes(nil)
	if result != nil {
		t.Fatalf("got %v, want nil", result)
	}
}

func TestCopyTypes_Single(t *testing.T) {
	input := []typ.Type{typ.String}
	result := extract.CopyTypes(input)

	if len(result) != 1 {
		t.Fatalf("got %d, want 1", len(result))
	}
	if result[0] != typ.String {
		t.Fatalf("got %v, want string", result[0])
	}
}

func TestCopyTypes_Multiple(t *testing.T) {
	input := []typ.Type{typ.String, typ.Integer, typ.Boolean}
	result := extract.CopyTypes(input)

	if len(result) != 3 {
		t.Fatalf("got %d, want 3", len(result))
	}

	input[0] = typ.Number
	if result[0] != typ.String {
		t.Fatal("copy should be independent of original")
	}
}

func TestSynthCallCore_FunctionWithArgs(t *testing.T) {
	fn := typ.Func().Param("a", typ.Integer).Param("b", typ.Integer).Returns(typ.Integer).Build()
	const sym = cfg.SymbolID(1)
	graph := mockGraph{symbols: map[string]cfg.SymbolID{"add": sym}}
	declared := flow.DeclaredTypes{sym: fn}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		DeclaredTypes: declared,
	})

	e := New(Config{
		Ctx:    db.NewQueryContext(db.New()),
		Types:  mockTypeQuerier{},
		Scopes: make(api.ScopeMap),
		Env:    checkCtx,
	})

	expr := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "add"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "1"},
			&ast.NumberExpr{Value: "2"},
		},
	}

	result := e.MultiTypeOf(expr, 0)
	if len(result) == 0 {
		t.Fatal("expected at least one return type")
	}
	if result[0].Kind() != kind.Integer && result[0].Kind() != kind.Unknown {
		t.Fatalf("got %v, want integer or unknown", result[0])
	}
}
