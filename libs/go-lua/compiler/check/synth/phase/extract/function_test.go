package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	ccfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

type countingGraphProvider struct {
	graph *ccfg.Graph
	calls int
}

func (p *countingGraphProvider) GetOrBuildCFG(*ast.FunctionExpr) *ccfg.Graph {
	p.calls++
	return p.graph
}

func TestSynthFunctionType_Nil(t *testing.T) {
	s := newTestSynthesizer()
	result := s.FunctionType(nil, nil)
	if result != nil {
		t.Fatal("expected nil for nil function")
	}
}

func TestSynthFunctionType_Empty(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()
	result := s.FunctionType(&ast.FunctionExpr{}, sc)
	if result == nil {
		t.Fatal("expected non-nil function")
	}
}

func TestSynthFunctionType_WithParams(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a", "b"},
			Types: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "string"},
				&ast.PrimitiveTypeExpr{Name: "number"},
			},
		},
	}

	result := s.FunctionType(fn, sc)
	if result == nil {
		t.Fatal("expected non-nil function")
	}
	if len(result.Params) != 2 {
		t.Fatalf("got %d params, want 2", len(result.Params))
	}
	if result.Params[0].Type != typ.String {
		t.Fatalf("got %v, want string", result.Params[0].Type)
	}
}

func TestSynthFunctionType_WithReturns(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		ReturnTypes: []ast.TypeExpr{
			&ast.PrimitiveTypeExpr{Name: "boolean"},
		},
	}

	result := s.FunctionType(fn, sc)
	if result == nil {
		t.Fatal("expected non-nil function")
	}
	if len(result.Returns) != 1 {
		t.Fatalf("got %d returns, want 1", len(result.Returns))
	}
	if result.Returns[0] != typ.Boolean {
		t.Fatalf("got %v, want boolean", result.Returns[0])
	}
}

func TestSynthFunctionType_Variadic(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names:      []string{"first"},
			HasVargs:   true,
			VarargType: &ast.PrimitiveTypeExpr{Name: "string"},
		},
	}

	result := s.FunctionType(fn, sc)
	if result == nil {
		t.Fatal("expected non-nil function")
	}
	if result.Variadic != typ.String {
		t.Fatalf("got %v, want string variadic", result.Variadic)
	}
}

func TestSynthFunctionType_NilScope(t *testing.T) {
	s := newTestSynthesizer()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
	}

	result := s.FunctionType(fn, nil)
	if result == nil {
		t.Fatal("expected non-nil function")
	}
}

func TestSynthFunctionTypeWithExpected_ParamInference(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()

	expected := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.Boolean).
		Build()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
	}

	result := s.SynthFunctionTypeWithExpected(fn, sc, expected)
	if result == nil {
		t.Fatal("expected non-nil function")
	}
	if len(result.Params) != 1 {
		t.Fatalf("got %d params, want 1", len(result.Params))
	}
	if result.Params[0].Type != typ.Integer {
		t.Fatalf("got %v, want integer from expected", result.Params[0].Type)
	}
}

func TestSynthFunctionTypeWithExpected_ReturnInference(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()

	expected := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.String).
		Build()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
	}

	result := s.SynthFunctionTypeWithExpected(fn, sc, expected)
	if result == nil {
		t.Fatal("expected non-nil function")
	}
	if len(result.Returns) != 1 {
		t.Fatalf("got %d returns, want 1", len(result.Returns))
	}
	if result.Returns[0] != typ.String {
		t.Fatalf("got %v, want string from expected", result.Returns[0])
	}
}

func TestSynthFunctionTypeWithExpected_VariadicInference(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()

	expected := typ.Func().
		Variadic(typ.Integer).
		Build()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
	}

	result := s.SynthFunctionTypeWithExpected(fn, sc, expected)
	if result == nil {
		t.Fatal("expected non-nil function")
	}
	if result.Variadic != typ.Integer {
		t.Fatalf("got %v, want integer variadic", result.Variadic)
	}
}

func TestSynthFunctionType_UsesAttachedGraphProvider(t *testing.T) {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.StringExpr{Value: "ok"}}},
		},
	}
	provider := &countingGraphProvider{graph: ccfg.Build(fn)}
	ctx := db.NewQueryContext(db.New())
	api.AttachGraphs(ctx, provider)

	s := NewSynthesizer(&Deps{
		Ctx:      ctx,
		Types:    mockTypeQuerier{},
		Scopes:   make(api.ScopeMap),
		Graphs:   provider,
		PreCache: make(api.Cache),
	}, api.PhaseTypeResolution)

	result := s.FunctionType(fn, scope.New())
	if result == nil {
		t.Fatal("expected non-nil function")
	}
	if provider.calls == 0 {
		t.Fatal("expected function synthesis to use attached graph provider")
	}
}

func TestSynthFunctionType_TypeParams(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()

	fn := &ast.FunctionExpr{
		TypeParams: []ast.TypeParamExpr{
			{Name: "T"},
		},
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{
				&ast.TypeRefExpr{Path: []string{"T"}},
			},
		},
	}

	result := s.FunctionType(fn, sc)
	if result == nil {
		t.Fatal("expected non-nil function")
	}
	if len(result.TypeParams) != 1 {
		t.Fatalf("got %d type params, want 1", len(result.TypeParams))
	}
}

func TestSynthFunctionType_OptionalParam(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{
				&ast.OptionalTypeExpr{Inner: &ast.PrimitiveTypeExpr{Name: "string"}},
			},
		},
	}

	result := s.FunctionType(fn, sc)
	if result == nil {
		t.Fatal("expected non-nil function")
	}
	if len(result.Params) != 1 {
		t.Fatalf("got %d params, want 1", len(result.Params))
	}
	if !result.Params[0].Optional {
		t.Fatal("expected optional param")
	}
}

func TestJoinTwo_Same(t *testing.T) {
	result := typ.JoinPreferNonSoft(typ.String, typ.String)
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestJoinTwo_Different(t *testing.T) {
	result := typ.JoinPreferNonSoft(typ.String, typ.Integer)
	union, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("got %T, want union", result)
	}
	if len(union.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(union.Members))
	}
}

func TestInferReturnExprTypes_Empty(t *testing.T) {
	s := newTestSynthesizer()
	result := s.inferReturnExprTypes(nil, 0)
	if result != nil {
		t.Fatal("expected nil for empty exprs")
	}
}

func TestInferReturnExprTypes_Single(t *testing.T) {
	s := newTestSynthesizer()
	exprs := []ast.Expr{&ast.StringExpr{Value: "hello"}}
	result := s.inferReturnExprTypes(exprs, 0)
	if len(result) != 1 {
		t.Fatalf("got %d types, want 1", len(result))
	}
}

func TestReturnInference_ArityUnion_ErrorObject(t *testing.T) {
	// Simulates:
	//   function get_db(db: string, err: integer?)
	//     if err then return nil, err end
	//     return db
	//   end
	// Expected: (string | nil, integer?)
	s := newTestSynthesizer()
	sc := scope.New()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"db", "err"},
			Types: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "string"},
				&ast.OptionalTypeExpr{Inner: &ast.PrimitiveTypeExpr{Name: "integer"}},
			},
		},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "err"},
				Then: []ast.Stmt{
					&ast.ReturnStmt{Exprs: []ast.Expr{
						&ast.NilExpr{},
						&ast.IdentExpr{Value: "err"},
					}},
				},
			},
			&ast.ReturnStmt{Exprs: []ast.Expr{
				&ast.IdentExpr{Value: "db"},
			}},
		},
	}

	result, _ := s.inferReturnTypesFromBody(fn, sc, nil, nil, 0, nil)
	if len(result) != 2 {
		t.Fatalf("got %d return types, want 2", len(result))
	}

	// Position 0: string | nil
	pos0 := result[0]
	opt0, ok := pos0.(*typ.Optional)
	if !ok {
		t.Fatalf("position 0: got %v (%T), want optional", pos0, pos0)
	}
	if opt0.Inner != typ.String {
		t.Fatalf("position 0: got optional(%v), want optional(string)", opt0.Inner)
	}

	// Position 1: integer? (merge of integer? and nil stays integer?)
	pos1 := result[1]
	opt1, ok := pos1.(*typ.Optional)
	if !ok {
		t.Fatalf("position 1: got %v (%T), want optional", pos1, pos1)
	}
	if opt1.Inner != typ.Integer {
		t.Fatalf("position 1: got optional(%v), want optional(integer)", opt1.Inner)
	}
}

func TestReturnInference_LastExprExpands(t *testing.T) {
	// Simulates:
	//   function f(foo: () -> (string, integer))
	//     return foo()
	//   end
	// Expected: 2 return slots [string, integer]
	s := newTestSynthesizer()
	sc := scope.New()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"foo"},
			Types: []ast.TypeExpr{
				&ast.FunctionTypeExpr{
					Returns: []ast.TypeExpr{
						&ast.PrimitiveTypeExpr{Name: "string"},
						&ast.PrimitiveTypeExpr{Name: "integer"},
					},
				},
			},
		},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{
				&ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "foo"},
				},
			}},
		},
	}

	result, _ := s.inferReturnTypesFromBody(fn, sc, nil, nil, 0, nil)
	if len(result) != 2 {
		t.Fatalf("got %d return types, want 2", len(result))
	}
	if result[0] != typ.String {
		t.Fatalf("position 0: got %v, want string", result[0])
	}
	if result[1] != typ.Integer {
		t.Fatalf("position 1: got %v, want integer", result[1])
	}
}

func TestReturnInference_ZeroReturn(t *testing.T) {
	// Simulates:
	//   function f(x: string)
	//     if cond then return end
	//     return x
	//   end
	// Expected: (string | nil)
	s := newTestSynthesizer()
	sc := scope.New()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "string"},
			},
		},
		Stmts: []ast.Stmt{
			&ast.IfStmt{
				Condition: &ast.TrueExpr{},
				Then: []ast.Stmt{
					&ast.ReturnStmt{},
				},
			},
			&ast.ReturnStmt{Exprs: []ast.Expr{
				&ast.IdentExpr{Value: "x"},
			}},
		},
	}

	result, _ := s.inferReturnTypesFromBody(fn, sc, nil, nil, 0, nil)
	if len(result) != 1 {
		t.Fatalf("got %d return types, want 1", len(result))
	}

	// string | nil
	opt, ok := result[0].(*typ.Optional)
	if !ok {
		t.Fatalf("got %v (%T), want optional", result[0], result[0])
	}
	if opt.Inner != typ.String {
		t.Fatalf("got optional(%v), want optional(string)", opt.Inner)
	}
}
