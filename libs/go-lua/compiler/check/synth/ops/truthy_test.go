package ops

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestCanBeFalsy_Primitives(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"nil", typ.Nil, true},
		{"boolean", typ.Boolean, true},
		{"any", typ.Any, true},
		{"unknown", typ.Unknown, true},
		{"integer", typ.Integer, false},
		{"number", typ.Number, false},
		{"string", typ.String, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanBeFalsy(tt.t); got != tt.want {
				t.Errorf("CanBeFalsy(%v) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestCanBeFalsy_Optional(t *testing.T) {
	opt := typ.NewOptional(typ.Integer)
	if !CanBeFalsy(opt) {
		t.Error("optional should be falsy")
	}
}

func TestCanBeFalsy_Union(t *testing.T) {
	u := typ.NewUnion(typ.Integer, typ.Nil)
	if !CanBeFalsy(u) {
		t.Error("union with nil should be falsy")
	}

	u2 := typ.NewUnion(typ.Integer, typ.String)
	if CanBeFalsy(u2) {
		t.Error("union without falsy member should not be falsy")
	}
}

func TestCanBeFalsy_Literal(t *testing.T) {
	falseLit := typ.LiteralBool(false)
	if !CanBeFalsy(falseLit) {
		t.Error("false literal should be falsy")
	}

	trueLit := typ.LiteralBool(true)
	if CanBeFalsy(trueLit) {
		t.Error("true literal should not be falsy")
	}
}

func TestCanBeFalsy_Alias(t *testing.T) {
	alias := &typ.Alias{Name: "MyOpt", Target: typ.NewOptional(typ.Integer)}
	if !CanBeFalsy(alias) {
		t.Error("alias of optional should be falsy")
	}
}

func TestCanBeFalsy_DeepAliasChainOptional(t *testing.T) {
	deep := makeAliasChain(typ.NewOptional(typ.Number), 40, "Opt")
	if !CanBeFalsy(deep) {
		t.Fatal("deep alias chain to optional should be falsy")
	}
}

func TestCanBeFalsy_Structures(t *testing.T) {
	rec := &typ.Record{Fields: []typ.Field{{Name: "x", Type: typ.Integer}}}
	if CanBeFalsy(rec) {
		t.Error("record should not be falsy")
	}

	arr := &typ.Array{Element: typ.Integer}
	if CanBeFalsy(arr) {
		t.Error("array should not be falsy")
	}

	m := &typ.Map{Key: typ.String, Value: typ.Integer}
	if CanBeFalsy(m) {
		t.Error("map should not be falsy")
	}
}

func TestIsFalsy(t *testing.T) {
	if !IsFalsy(typ.Nil) {
		t.Error("nil should be falsy")
	}

	if !IsFalsy(typ.LiteralBool(false)) {
		t.Error("false literal should be falsy")
	}

	if IsFalsy(typ.Integer) {
		t.Error("integer should not be falsy")
	}

	if IsFalsy(typ.LiteralBool(true)) {
		t.Error("true literal should not be falsy")
	}
}

func TestIsTruthy(t *testing.T) {
	if !IsTruthy(typ.Integer) {
		t.Error("integer should be truthy")
	}

	if !IsTruthy(typ.String) {
		t.Error("string should be truthy")
	}

	if IsTruthy(typ.Nil) {
		t.Error("nil should not be truthy")
	}

	if IsTruthy(typ.NewOptional(typ.Integer)) {
		t.Error("optional should not be truthy")
	}
}

func TestExtractFirstValue(t *testing.T) {
	if ExtractFirstValue(nil) != nil {
		t.Error("nil should return nil")
	}

	if ExtractFirstValue(typ.Integer) != typ.Integer {
		t.Error("non-tuple should return itself")
	}

	tuple := &typ.Tuple{Elements: []typ.Type{typ.String, typ.Integer}}
	if ExtractFirstValue(tuple) != typ.String {
		t.Error("tuple should return first element")
	}

	emptyTuple := &typ.Tuple{}
	if ExtractFirstValue(emptyTuple) != emptyTuple {
		t.Error("empty tuple should return itself")
	}
}

func TestCanBeFalsy_Nil(t *testing.T) {
	if CanBeFalsy(nil) {
		t.Error("nil type should return false")
	}
}
