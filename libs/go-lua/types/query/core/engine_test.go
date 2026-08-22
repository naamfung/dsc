package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewEngine(t *testing.T) {
	e := NewEngine()
	if e == nil {
		t.Fatal("NewEngine returned nil")
	}
	if e.fieldQ == nil {
		t.Error("fieldQ should be initialized")
	}
	if e.methodQ == nil {
		t.Error("methodQ should be initialized")
	}
	if e.indexQ == nil {
		t.Error("indexQ should be initialized")
	}
}

func TestNewEngineWithStdlib(t *testing.T) {
	cfg := StdlibConfig{
		MethodProviders: map[kind.Kind]*typ.Record{
			kind.String: typ.NewRecord().Field("len", typ.Func().Returns(typ.Integer).Build()).Build(),
		},
	}
	e := NewEngineWithStdlib(cfg)
	if e == nil {
		t.Fatal("NewEngineWithStdlib returned nil")
	}
	if len(e.methodProviders) != 1 {
		t.Error("method providers should be set")
	}
}

func TestEngine_Field_Record(t *testing.T) {
	e := NewEngine()
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	fieldType, ok := e.Field(nil, rec, "x")
	if !ok {
		t.Fatal("Field should find 'x'")
	}
	if fieldType != typ.Number {
		t.Errorf("got %v, want number", fieldType)
	}
}

func TestEngine_Field_NotFound(t *testing.T) {
	e := NewEngine()
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	_, ok := e.Field(nil, rec, "y")
	if ok {
		t.Error("Field should not find 'y'")
	}
}

func TestEngine_Index_Array(t *testing.T) {
	e := NewEngine()
	arr := typ.NewArray(typ.String)
	elemType, ok := e.Index(nil, arr, typ.Integer)
	if !ok {
		t.Fatal("Index should succeed for array")
	}
	if elemType != typ.String {
		t.Errorf("got %v, want string", elemType)
	}
}

func TestEngine_Index_Map(t *testing.T) {
	e := NewEngine()
	m := typ.NewMap(typ.String, typ.Number)
	valType, ok := e.Index(nil, m, typ.String)
	if !ok {
		t.Fatal("Index should succeed for map")
	}
	if valType == nil {
		t.Error("expected non-nil value type")
	}
}

func TestEngine_UnaryOp_Not(t *testing.T) {
	e := NewEngine()
	result := e.UnaryOp(nil, "not", typ.Boolean)
	if result != typ.Boolean {
		t.Errorf("not boolean should be boolean, got %v", result)
	}
}

func TestEngine_UnaryOp_Minus(t *testing.T) {
	e := NewEngine()
	result := e.UnaryOp(nil, "-", typ.Number)
	if result != typ.Number {
		t.Errorf("-number should be number, got %v", result)
	}
}

func TestEngine_BinaryOp_Add(t *testing.T) {
	e := NewEngine()
	result := e.BinaryOp(nil, typ.Number, "+", typ.Number)
	if result != typ.Number {
		t.Errorf("number + number should be number, got %v", result)
	}
}

func TestEngine_BinaryOp_Concat(t *testing.T) {
	e := NewEngine()
	result := e.BinaryOp(nil, typ.String, "..", typ.String)
	if result != typ.String {
		t.Errorf("string .. string should be string, got %v", result)
	}
}

func TestEngine_Callable_Function(t *testing.T) {
	e := NewEngine()
	fn := typ.Func().Returns(typ.String).Build()
	result, ok := e.Callable(nil, fn)
	if !ok {
		t.Fatal("function should be callable")
	}
	if result == nil {
		t.Error("should return function type")
	}
}

func TestEngine_IsSubtype_RecursiveAliasRecordUnion(t *testing.T) {
	e := NewEngine()
	ctx := db.NewQueryContext(db.New())

	rec := typ.NewRecursivePlaceholder("Message")
	msgAlias := typ.NewAlias("Message", rec)
	rec.SetBody(typ.NewRecord().
		Field("_topic", typ.String).
		Field("topic", typ.Func().Param("self", rec).Returns(typ.String).Build()).
		Build())

	msgCh := typ.NewAlias("MsgCh", typ.NewRecord().Field("__tag", typ.LiteralString("msg")).Build())
	timerCh := typ.NewAlias("TimerCh", typ.NewRecord().Field("__tag", typ.LiteralString("timer")).Build())
	timer := typ.NewRecord().Field("elapsed", typ.Number).Build()

	result := typ.NewUnion(
		typ.NewRecord().
			Field("channel", msgCh).
			Field("value", msgAlias).
			Field("ok", typ.Boolean).
			Build(),
		typ.NewRecord().
			Field("channel", timerCh).
			Field("value", timer).
			Field("ok", typ.Boolean).
			Build(),
	)

	synthesized := typ.NewRecord().
		Field("channel", msgCh).
		Field("value", typ.NewRecord().
			Field("_topic", typ.String).
			Field("topic", typ.Func().Param("s", msgAlias).Returns(typ.String).Build()).
			Build()).
		Field("ok", typ.True).
		Build()

	if !e.IsSubtype(ctx, synthesized, result) {
		t.Fatal("engine subtype should accept recursive alias record inside union member")
	}
}

func TestEngine_Callable_NonFunction(t *testing.T) {
	e := NewEngine()
	_, ok := e.Callable(nil, typ.String)
	if ok {
		t.Error("string should not be callable")
	}
}
