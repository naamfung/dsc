package intercept

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSelectIntercept_NotSelectCall_SkipsFalse(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "print"},
		Args: []ast.Expr{&ast.StringExpr{Value: "hello"}},
	}
	result := s.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for non-select call")
	}
}

func TestSelectIntercept_NoArgs_SkipsFalse(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{},
	}
	result := s.InterceptCall(ex, CallEnv{})
	if result.Skip {
		t.Fatal("expected skip=false for no args")
	}
}

func TestSelectIntercept_HashPattern_ReturnsInteger(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{&ast.StringExpr{Value: "#"}},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
	}
	result := s.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for select('#')")
	}
	if len(result.Types) != 1 {
		t.Fatal("expected one type")
	}
	if result.Types[0] != typ.Integer {
		t.Fatal("expected integer type")
	}
}

func TestSelectIntercept_OnlyFirstArg_SkipsFalse(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{&ast.NumberExpr{Value: "1"}},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
	}
	result := s.InterceptCall(ex, ctx)
	if result.Skip {
		t.Fatal("expected skip=false for only first arg")
	}
}

func TestSelectIntercept_VarargWithResolver_UsesResolverType(t *testing.T) {
	resolver := &selectTestVariadicResolver{varType: typ.Number}
	s := &SelectIntercept{VariadicResolver: resolver}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "1"},
			&ast.Comma3Expr{},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
	}
	result := s.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for vararg pattern")
	}
	if result.Types[0] != typ.Number {
		t.Fatal("expected number type from resolver")
	}
}

func TestSelectIntercept_VarargWithoutResolver_SkipsFalse(t *testing.T) {
	s := &SelectIntercept{VariadicResolver: nil}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "1"},
			&ast.Comma3Expr{},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
	}
	result := s.InterceptCall(ex, ctx)
	if result.Skip {
		t.Fatal("expected skip=false when resolver is nil")
	}
}

func TestSelectIntercept_ConcreteIndex_ReturnsTailTypes(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "2"},
			&ast.StringExpr{Value: "first"},
			&ast.StringExpr{Value: "second"},
			&ast.NumberExpr{Value: "3"},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		Recurse: func(e ast.Expr) typ.Type {
			if _, ok := e.(*ast.StringExpr); ok {
				return typ.String
			}
			if _, ok := e.(*ast.NumberExpr); ok {
				return typ.Number
			}
			return typ.Unknown
		},
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
	}
	result := s.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for concrete index")
	}
	if len(result.Types) != 2 {
		t.Fatalf("expected 2 tail types, got %d", len(result.Types))
	}
	if result.Types[0] != typ.String || result.Types[1] != typ.Number {
		t.Fatalf("expected [string, number], got [%v, %v]", result.Types[0], result.Types[1])
	}
}

func TestSelectIntercept_IntegralFloatIndex_ReturnsTailTypes(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "1e0"},
			&ast.StringExpr{Value: "first"},
			&ast.NumberExpr{Value: "3"},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		Recurse: func(e ast.Expr) typ.Type {
			if _, ok := e.(*ast.StringExpr); ok {
				return typ.String
			}
			if _, ok := e.(*ast.NumberExpr); ok {
				return typ.Number
			}
			return typ.Unknown
		},
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
	}
	result := s.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for integral float index")
	}
	if len(result.Types) != 2 {
		t.Fatalf("expected 2 tail types, got %d", len(result.Types))
	}
	if result.Types[0] != typ.String || result.Types[1] != typ.Number {
		t.Fatalf("expected [string, number], got [%v, %v]", result.Types[0], result.Types[1])
	}
}

func TestSelectIntercept_IndexOutOfRange_SkipsFalse(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "10"},
			&ast.StringExpr{Value: "only"},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
	}
	result := s.InterceptCall(ex, ctx)
	if result.Skip {
		t.Fatal("expected skip=false for out of range index")
	}
}

func TestSelectIntercept_NegativeIndex(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "-1"},
			&ast.StringExpr{Value: "first"},
			&ast.StringExpr{Value: "second"},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		Recurse: func(e ast.Expr) typ.Type {
			if _, ok := e.(*ast.StringExpr); ok {
				return typ.String
			}
			return typ.Unknown
		},
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
	}
	result := s.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for negative index")
	}
	if len(result.Types) != 1 || result.Types[0] != typ.String {
		t.Fatal("expected string type for negative index selection")
	}
}

func TestSelectIntercept_NegativeIndex_ReturnsTailTypes(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "-2"},
			&ast.StringExpr{Value: "first"},
			&ast.NumberExpr{Value: "2"},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		Recurse: func(e ast.Expr) typ.Type {
			if _, ok := e.(*ast.StringExpr); ok {
				return typ.String
			}
			if _, ok := e.(*ast.NumberExpr); ok {
				return typ.Number
			}
			return typ.Unknown
		},
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
	}
	result := s.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for negative index")
	}
	if len(result.Types) != 2 {
		t.Fatalf("expected 2 tail types, got %d", len(result.Types))
	}
	if result.Types[0] != typ.String || result.Types[1] != typ.Number {
		t.Fatalf("expected [string, number], got [%v, %v]", result.Types[0], result.Types[1])
	}
}

func TestSelectIntercept_NegativeIntegralFloatIndex(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "-1.0"},
			&ast.StringExpr{Value: "first"},
			&ast.NumberExpr{Value: "2"},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		Recurse: func(e ast.Expr) typ.Type {
			if _, ok := e.(*ast.StringExpr); ok {
				return typ.String
			}
			if _, ok := e.(*ast.NumberExpr); ok {
				return typ.Number
			}
			return typ.Unknown
		},
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
	}
	result := s.InterceptCall(ex, ctx)
	if !result.Skip {
		t.Fatal("expected skip=true for negative integral float index")
	}
	if len(result.Types) != 1 || result.Types[0] != typ.Number {
		t.Fatalf("expected [number], got %v", result.Types)
	}
}

func TestSelectIntercept_ZeroIndex(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "0"},
			&ast.StringExpr{Value: "value"},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
	}
	result := s.InterceptCall(ex, ctx)
	if result.Skip {
		t.Fatal("expected skip=false for zero index")
	}
}

func TestSelectIntercept_FractionalIndex(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "1.5"},
			&ast.StringExpr{Value: "a"},
			&ast.StringExpr{Value: "b"},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
		Recurse: func(ast.Expr) typ.Type { return typ.String },
	}
	result := s.InterceptCall(ex, ctx)
	if result.Skip {
		t.Fatal("expected skip=false for fractional index")
	}
}

func TestSelectIntercept_NegativeIndexOutOfRange(t *testing.T) {
	s := &SelectIntercept{}
	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "select"},
		Args: []ast.Expr{
			&ast.NumberExpr{Value: "-2"},
			&ast.StringExpr{Value: "only"},
		},
	}
	selectFn := typ.Func().
		Param("index", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Any).
		Effects(effect.WithVariadicTransform()).
		Build()
	ctx := CallEnv{
		TypeLookup: func(name string) typ.Type {
			if name == "select" {
				return selectFn
			}
			return nil
		},
		Recurse: func(ast.Expr) typ.Type { return typ.String },
	}
	result := s.InterceptCall(ex, ctx)
	if result.Skip {
		t.Fatal("expected skip=false for negative index out of range")
	}
}

func TestIsSelectCall_NilExpr_ReturnsFalse(t *testing.T) {
	if isSelectCall(nil, CallEnv{}) {
		t.Fatal("expected false for nil expr")
	}
}

func TestIsSelectCall_NilFunc_ReturnsFalse(t *testing.T) {
	ex := &ast.FuncCallExpr{Func: nil}
	if isSelectCall(ex, CallEnv{}) {
		t.Fatal("expected false for nil func")
	}
}

func TestVariadicTypeResolver_Interface(t *testing.T) {
	var _ VariadicTypeResolver = (*selectTestVariadicResolver)(nil)
}

type selectTestVariadicResolver struct {
	varType typ.Type
}

func (r *selectTestVariadicResolver) VariadicType() typ.Type {
	return r.varType
}
