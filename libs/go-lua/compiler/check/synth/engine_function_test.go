package synth

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSynthFunctionType_Nil(t *testing.T) {
	e := newTestEngine()
	result := e.FunctionType(nil, nil)
	if result != nil {
		t.Fatalf("got %v, want nil", result)
	}
}

func TestSynthFunctionType_Empty(t *testing.T) {
	e := newTestEngine()
	fn := &ast.FunctionExpr{}
	sc := scope.New()

	result := e.FunctionType(fn, sc)
	if result == nil {
		t.Fatal("got nil, want function")
	}
	if result.Kind() != kind.Function {
		t.Fatalf("got %v, want function", result.Kind())
	}
}

func TestSynthFunctionType_SingleParam(t *testing.T) {
	e := newTestEngine()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
		},
	}
	sc := scope.New()

	result := e.FunctionType(fn, sc)
	if len(result.Params) != 1 {
		t.Fatalf("got %d params, want 1", len(result.Params))
	}
	if result.Params[0].Name != "x" {
		t.Fatalf("param name: got %q, want %q", result.Params[0].Name, "x")
	}
	if result.Params[0].Type != typ.Number {
		t.Fatalf("param type: got %v, want number", result.Params[0].Type)
	}
}

func TestSynthFunctionType_MultipleParams(t *testing.T) {
	e := newTestEngine()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a", "b", "c"},
			Types: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "string"},
				&ast.PrimitiveTypeExpr{Name: "number"},
				&ast.PrimitiveTypeExpr{Name: "boolean"},
			},
		},
	}
	sc := scope.New()

	result := e.FunctionType(fn, sc)
	if len(result.Params) != 3 {
		t.Fatalf("got %d params, want 3", len(result.Params))
	}
}

func TestSynthFunctionType_OptionalParam(t *testing.T) {
	e := newTestEngine()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{
				&ast.OptionalTypeExpr{Inner: &ast.PrimitiveTypeExpr{Name: "string"}},
			},
		},
	}
	sc := scope.New()

	result := e.FunctionType(fn, sc)
	if len(result.Params) != 1 {
		t.Fatalf("got %d params, want 1", len(result.Params))
	}
	if !result.Params[0].Optional {
		t.Fatal("param should be optional")
	}
}

func TestSynthFunctionType_Variadic(t *testing.T) {
	e := newTestEngine()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names:      []string{"a"},
			Types:      []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "string"}},
			HasVargs:   true,
			VarargType: &ast.PrimitiveTypeExpr{Name: "number"},
		},
	}
	sc := scope.New()

	result := e.FunctionType(fn, sc)
	if result.Variadic == nil {
		t.Fatal("expected variadic type")
	}
	if result.Variadic != typ.Number {
		t.Fatalf("variadic type: got %v, want number", result.Variadic)
	}
}

func TestSynthFunctionType_VariadicNoType(t *testing.T) {
	e := newTestEngine()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			HasVargs: true,
		},
	}
	sc := scope.New()

	result := e.FunctionType(fn, sc)
	if result.Variadic == nil {
		t.Fatal("expected variadic type")
	}
	if result.Variadic != typ.Any {
		t.Fatalf("variadic type: got %v, want any", result.Variadic)
	}
}

func TestSynthFunctionType_Returns(t *testing.T) {
	e := newTestEngine()
	fn := &ast.FunctionExpr{
		ReturnTypes: []ast.TypeExpr{
			&ast.PrimitiveTypeExpr{Name: "string"},
			&ast.PrimitiveTypeExpr{Name: "number"},
		},
	}
	sc := scope.New()

	result := e.FunctionType(fn, sc)
	if len(result.Returns) != 2 {
		t.Fatalf("got %d returns, want 2", len(result.Returns))
	}
	if result.Returns[0] != typ.String {
		t.Fatalf("return 0: got %v, want string", result.Returns[0])
	}
	if result.Returns[1] != typ.Number {
		t.Fatalf("return 1: got %v, want number", result.Returns[1])
	}
}

func TestSynthFunctionType_TypeParams(t *testing.T) {
	e := newTestEngine()
	fn := &ast.FunctionExpr{
		TypeParams: []ast.TypeParamExpr{
			{Name: "T"},
			{Name: "U", Constraint: &ast.PrimitiveTypeExpr{Name: "number"}},
		},
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"T"}}},
		},
		ReturnTypes: []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"T"}}},
	}
	sc := scope.New()

	result := e.FunctionType(fn, sc)
	if len(result.TypeParams) != 2 {
		t.Fatalf("got %d type params, want 2", len(result.TypeParams))
	}
	if result.TypeParams[0].Name != "T" {
		t.Fatalf("type param 0: got %q, want %q", result.TypeParams[0].Name, "T")
	}
	if result.TypeParams[1].Constraint == nil {
		t.Fatal("type param 1 should have constraint")
	}
}

func TestResolveReturnTypes_Empty(t *testing.T) {
	e := newTestEngine()
	result := e.ResolveReturnTypes(nil, nil)
	if result != nil {
		t.Fatalf("got %v, want nil", result)
	}
}

func TestResolveReturnTypes_Single(t *testing.T) {
	e := newTestEngine()
	types := []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "string"}}
	sc := scope.New()

	result := e.ResolveReturnTypes(types, sc)
	if len(result) != 1 {
		t.Fatalf("got %d, want 1", len(result))
	}
	if result[0] != typ.String {
		t.Fatalf("got %v, want string", result[0])
	}
}

func TestResolveReturnTypes_Tuple(t *testing.T) {
	e := newTestEngine()
	types := []ast.TypeExpr{
		&ast.TupleTypeExpr{
			Elements: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "string"},
				&ast.PrimitiveTypeExpr{Name: "number"},
			},
		},
	}
	sc := scope.New()

	result := e.ResolveReturnTypes(types, sc)
	if len(result) != 2 {
		t.Fatalf("got %d, want 2", len(result))
	}
	if result[0] != typ.String {
		t.Fatalf("got %v, want string", result[0])
	}
	if result[1] != typ.Number {
		t.Fatalf("got %v, want number", result[1])
	}
}

func TestResolveReturnTypes_Mixed(t *testing.T) {
	e := newTestEngine()
	types := []ast.TypeExpr{
		&ast.PrimitiveTypeExpr{Name: "boolean"},
		&ast.TupleTypeExpr{
			Elements: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "string"},
				&ast.PrimitiveTypeExpr{Name: "number"},
			},
		},
	}
	sc := scope.New()

	result := e.ResolveReturnTypes(types, sc)
	if len(result) != 3 {
		t.Fatalf("got %d, want 3", len(result))
	}
	if result[0] != typ.Boolean {
		t.Fatalf("result[0]: got %v, want boolean", result[0])
	}
}

func TestSynthFunctionType_NoScope(t *testing.T) {
	e := newTestEngine()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x"},
			Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
		},
	}

	result := e.FunctionType(fn, nil)
	if result == nil {
		t.Fatal("should handle nil scope")
	}
}

func TestSynthFunctionType_UntypedParam(t *testing.T) {
	e := newTestEngine()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"x", "y"},
			Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
		},
	}
	sc := scope.New()

	result := e.FunctionType(fn, sc)
	if len(result.Params) != 2 {
		t.Fatalf("got %d params, want 2", len(result.Params))
	}
	if result.Params[1].Type != typ.Unknown {
		t.Fatalf("untyped param: got %v, want unknown", result.Params[1].Type)
	}
}

// TestInferReturnTypesFromBody_CallbackReturn tests that return type inference
// works correctly when the function returns the result of calling a callback parameter.
func TestInferReturnTypesFromBody_CallbackReturn(t *testing.T) {
	// Test: function(f: (number) -> number) return f(10) end
	// Should infer return type as: number
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"f"},
			Types: []ast.TypeExpr{
				&ast.FunctionTypeExpr{
					Params:  []ast.FunctionParamExpr{{Name: "x", Type: &ast.PrimitiveTypeExpr{Name: "number"}}},
					Returns: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
				},
			},
		},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{
					&ast.FuncCallExpr{
						Func: &ast.IdentExpr{Value: "f"},
						Args: []ast.Expr{&ast.NumberExpr{Value: "10"}},
					},
				},
			},
		},
	}

	e := newTestEngine()
	sc := scope.New()

	result := e.FunctionType(fn, sc)
	if result == nil {
		t.Fatal("expected function type, got nil")
	}
	if len(result.Returns) == 0 {
		t.Fatal("expected inferred return type, got none")
	}
	if result.Returns[0] != typ.Number {
		t.Errorf("expected return type number, got %v", result.Returns[0])
	}
}
