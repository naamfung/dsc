package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewCallPipeline(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := ops.CallDef{
		Callee: typ.Func().Build(),
	}
	args := []ast.Expr{&ast.NumberExpr{Value: "1"}}

	pipeline := NewCallPipeline(ctx, def, args)
	if pipeline == nil {
		t.Fatal("expected non-nil pipeline")
	}
	if pipeline.ctx != ctx {
		t.Fatal("context mismatch")
	}
}

func TestCallPipeline_WithReSynth(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := ops.CallDef{
		Callee: typ.Func().Build(),
	}
	pipeline := NewCallPipeline(ctx, def, nil)

	reSynth := func(idx int, arg ast.Expr, expected typ.Type) typ.Type {
		return typ.String
	}

	result := pipeline.WithReSynth(reSynth)
	if result != pipeline {
		t.Fatal("expected same pipeline returned")
	}
	if pipeline.reSynth == nil {
		t.Fatal("expected reSynth to be set")
	}
}

func TestCallPipeline_WithExpected(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := ops.CallDef{
		Callee: typ.Func().Build(),
	}
	pipeline := NewCallPipeline(ctx, def, nil)

	expected := typ.String
	result := pipeline.WithExpected(expected)
	if result != pipeline {
		t.Fatal("expected same pipeline returned")
	}
	if pipeline.def.ExpectedReturn != expected {
		t.Fatal("expected return type not set")
	}
}

func TestCallPipeline_Infer(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.String).
		Build()
	def := ops.CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}
	pipeline := NewCallPipeline(ctx, def, nil)

	infer := pipeline.Infer()
	if infer.Callee == nil {
		t.Fatal("expected callee to be resolved")
	}
}

func TestCallPipeline_ExpectedArgType_InRange(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().
		Param("x", typ.Integer).
		Param("y", typ.String).
		Build()
	def := ops.CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer, typ.String},
	}
	pipeline := NewCallPipeline(ctx, def, nil)
	pipeline.Infer()

	arg0 := pipeline.ExpectedArgType(0)
	if arg0 != typ.Integer {
		t.Fatalf("got %v, want integer", arg0)
	}
	arg1 := pipeline.ExpectedArgType(1)
	if arg1 != typ.String {
		t.Fatalf("got %v, want string", arg1)
	}
}

func TestCallPipeline_ExpectedArgType_OutOfRange(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().
		Param("x", typ.Integer).
		Variadic(typ.String).
		Build()
	def := ops.CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer, typ.String, typ.String},
	}
	pipeline := NewCallPipeline(ctx, def, nil)
	pipeline.Infer()

	arg5 := pipeline.ExpectedArgType(5)
	if arg5 != typ.String {
		t.Fatalf("got %v, want string (variadic)", arg5)
	}
}

func TestCallPipeline_ReSynthAndReInfer_NoReSynth(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().Build()
	def := ops.CallDef{Callee: fn}
	pipeline := NewCallPipeline(ctx, def, nil)
	pipeline.Infer()

	changed := pipeline.ReSynthAndReInfer()
	if changed {
		t.Fatal("expected no change without reSynth")
	}
}

func TestCallPipeline_ReSynthAndReInfer_NoArgs(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().Build()
	def := ops.CallDef{Callee: fn}
	pipeline := NewCallPipeline(ctx, def, nil)
	pipeline.WithReSynth(func(idx int, arg ast.Expr, expected typ.Type) typ.Type {
		return typ.String
	})
	pipeline.Infer()

	changed := pipeline.ReSynthAndReInfer()
	if changed {
		t.Fatal("expected no change without args")
	}
}

func TestCallPipeline_Finish(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().Returns(typ.String).Build()
	def := ops.CallDef{
		Callee: fn,
		Args:   []typ.Type{},
	}
	pipeline := NewCallPipeline(ctx, def, nil)
	pipeline.Infer()

	result := pipeline.Finish()
	if result.Type == nil {
		t.Fatal("expected non-nil result type")
	}
	if !pipeline.finished {
		t.Fatal("expected finished flag to be set")
	}
}

func TestCallPipeline_Run(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	fn := typ.Func().Returns(typ.Integer).Build()
	def := ops.CallDef{
		Callee: fn,
		Args:   []typ.Type{},
	}
	pipeline := NewCallPipeline(ctx, def, nil)

	result := pipeline.Run()
	if result.Type == nil {
		t.Fatal("expected non-nil result type")
	}
}

func TestFunctionLiteralReSynth_NotFunction(t *testing.T) {
	reSynth := FunctionLiteralReSynth(func(fn *ast.FunctionExpr, expected *typ.Function) typ.Type {
		return typ.String
	})

	result := reSynth(0, &ast.NumberExpr{Value: "1"}, typ.Integer)
	if result != nil {
		t.Fatal("expected nil for non-function arg")
	}
}

func TestFunctionLiteralReSynth_NotFunctionExpected(t *testing.T) {
	reSynth := FunctionLiteralReSynth(func(fn *ast.FunctionExpr, expected *typ.Function) typ.Type {
		return typ.String
	})

	result := reSynth(0, &ast.FunctionExpr{}, typ.Integer)
	if result != nil {
		t.Fatal("expected nil for non-function expected")
	}
}

func TestFunctionLiteralReSynth_Match(t *testing.T) {
	called := false
	reSynth := FunctionLiteralReSynth(func(fn *ast.FunctionExpr, expected *typ.Function) typ.Type {
		called = true
		return typ.String
	})

	fnExpr := &ast.FunctionExpr{}
	expectedFn := typ.Func().Build()
	result := reSynth(0, fnExpr, expectedFn)

	if !called {
		t.Fatal("expected callback to be called")
	}
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestFullArgReSynth_Function(t *testing.T) {
	called := false
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		called = true
		return typ.String
	}

	reSynth := FullArgReSynth(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.FunctionExpr{}, typ.Func().Build())

	if !called {
		t.Fatal("expected callback to be called")
	}
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestFullArgReSynth_Table(t *testing.T) {
	called := false
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		called = true
		return typ.String
	}

	reSynth := FullArgReSynth(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.TableExpr{}, typ.NewRecord().Build())

	if !called {
		t.Fatal("expected callback to be called")
	}
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestFullArgReSynth_Other(t *testing.T) {
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		return typ.String
	}

	reSynth := FullArgReSynth(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.NumberExpr{}, typ.Integer)

	if result != nil {
		t.Fatal("expected nil for non-function/table")
	}
}

func TestFullArgReSynth_Identifier(t *testing.T) {
	called := false
	synthWithExpected := func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
		called = true
		return typ.String
	}

	reSynth := FullArgReSynth(synthWithExpected, nil, 0)
	result := reSynth(0, &ast.IdentExpr{Value: "cb"}, typ.Func().Build())

	if !called {
		t.Fatal("expected callback to be called for identifier")
	}
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}
