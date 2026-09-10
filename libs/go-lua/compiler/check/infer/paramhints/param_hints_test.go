package paramhints

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestWidenParamHintType_Nil(t *testing.T) {
	result := WidenParamHintType(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestWidenParamHintType_BooleanLiteral(t *testing.T) {
	lit := typ.LiteralBool(true)
	result := WidenParamHintType(lit)
	if result != typ.Boolean {
		t.Errorf("expected Boolean, got %v", result)
	}
}

func TestWidenParamHintType_IntegerLiteral(t *testing.T) {
	lit := typ.LiteralInt(42)
	result := WidenParamHintType(lit)
	if result != typ.Integer {
		t.Errorf("expected Integer, got %v", result)
	}
}

func TestWidenParamHintType_NumberLiteral(t *testing.T) {
	lit := typ.LiteralNumber(3.14)
	result := WidenParamHintType(lit)
	if result != typ.Number {
		t.Errorf("expected Number, got %v", result)
	}
}

func TestWidenParamHintType_StringLiteral(t *testing.T) {
	lit := typ.LiteralString("hello")
	result := WidenParamHintType(lit)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestWidenParamHintType_NonLiteral(t *testing.T) {
	result := WidenParamHintType(typ.String)
	if result != typ.String {
		t.Errorf("expected String unchanged, got %v", result)
	}
}

func TestWidenParamHintType_Alias(t *testing.T) {
	alias := typ.NewAlias("NumAlias", typ.Number)
	result := WidenParamHintType(alias)
	if result != typ.Number {
		t.Errorf("expected alias to widen to Number, got %v", result)
	}
}

func TestWidenParamHintType_Optional(t *testing.T) {
	lit := typ.LiteralString("hello")
	opt := typ.NewOptional(lit)
	result := WidenParamHintType(opt)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	optResult, ok := result.(*typ.Optional)
	if !ok {
		t.Fatalf("expected Optional, got %T", result)
	}
	if optResult.Inner != typ.String {
		t.Errorf("expected inner to be String, got %v", optResult.Inner)
	}
}

func TestWidenParamHintType_Union(t *testing.T) {
	lit1 := typ.LiteralString("a")
	lit2 := typ.LiteralNumber(1.0)
	union := typ.NewUnion(lit1, lit2)
	result := WidenParamHintType(union)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestWidenParamHintType_RecordBecomesOpen(t *testing.T) {
	rec := typ.NewRecord().
		Field("pid", typ.LiteralString("abc")).
		Field("topic", typ.LiteralString("test:update")).
		Build()

	result := WidenParamHintType(rec)
	widened, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", result)
	}
	if !widened.Open {
		t.Fatalf("expected widened param hint record to be open, got closed: %v", widened)
	}

	pid := widened.GetField("pid")
	if pid == nil || !typ.TypeEquals(pid.Type, typ.String) {
		t.Fatalf("expected pid field widened to string, got %v", pid)
	}
	topic := widened.GetField("topic")
	if topic == nil || !typ.TypeEquals(topic.Type, typ.String) {
		t.Fatalf("expected topic field widened to string, got %v", topic)
	}
}

func TestBuildParamHintSigView_NilInputs(t *testing.T) {
	result := BuildParamHintSigView(nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil inputs, got %v", result)
	}
}

func TestIsInformativeHintType(t *testing.T) {
	tests := []struct {
		name string
		in   typ.Type
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "any", in: typ.Any, want: false},
		{name: "unknown", in: typ.Unknown, want: false},
		{name: "never", in: typ.Never, want: false},
		{name: "nil type", in: typ.Nil, want: false},
		{name: "empty record", in: typ.NewRecord().Build(), want: false},
		{name: "map with string key", in: typ.NewMap(typ.String, typ.NewArray(typ.Any)), want: true},
		{name: "record map component", in: typ.NewRecord().MapComponent(typ.String, typ.Any).Build(), want: true},
		{name: "string", in: typ.String, want: true},
		{name: "literal", in: typ.LiteralString("x"), want: true},
		{name: "type param", in: typ.NewTypeParam("T", nil), want: false},
		{name: "ref", in: typ.NewRef("", "Foo"), want: false},
		{name: "optional unknown", in: typ.NewOptional(typ.Unknown), want: false},
		{name: "optional string", in: typ.NewOptional(typ.String), want: true},
		{name: "union placeholders", in: typ.NewUnion(typ.Unknown, typ.Nil), want: false},
		{name: "union with informative member", in: typ.NewUnion(typ.Unknown, typ.String), want: true},
	}

	for _, tt := range tests {
		if got := IsInformativeHintType(tt.in); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestEnsureHintCapacity(t *testing.T) {
	base := []typ.Type{typ.String}
	got := EnsureHintCapacity(base, 3)
	if len(got) != 3 {
		t.Fatalf("EnsureHintCapacity len = %d, want 3", len(got))
	}
	if got[0] != typ.String {
		t.Fatalf("EnsureHintCapacity preserved value = %v, want string", got[0])
	}
}

func TestMergeHintAt(t *testing.T) {
	join := func(prev, next typ.Type) typ.Type { return typ.JoinPreferNonSoft(prev, next) }

	t.Run("filters non-informative", func(t *testing.T) {
		hints := []typ.Type{typ.String}
		got, changed := MergeHintAt(hints, 1, typ.Unknown, join)
		if changed {
			t.Fatal("expected no change for unknown hint")
		}
		if len(got) != 1 {
			t.Fatalf("expected unchanged slice len 1, got %d", len(got))
		}
	})

	t.Run("normalizes literal and merges", func(t *testing.T) {
		got, changed := MergeHintAt(nil, 0, typ.LiteralString("x"), join)
		if !changed {
			t.Fatal("expected merge change for informative literal")
		}
		if len(got) != 1 {
			t.Fatalf("expected one hint, got %d", len(got))
		}
		if !typ.TypeEquals(got[0], typ.String) {
			t.Fatalf("expected normalized string hint, got %v", got[0])
		}
	})
}
