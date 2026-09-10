package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

func TestLiteralBool(t *testing.T) {
	tr := LiteralBool(true)
	fa := LiteralBool(false)

	if tr.Kind() != kind.Literal {
		t.Errorf("Kind: got %v, want Literal", tr.Kind())
	}

	if tr.Base != kind.Boolean {
		t.Errorf("Base: got %v, want Boolean", tr.Base)
	}

	if tr.Value != true {
		t.Error("Value should be true")
	}

	if tr.String() != "true" {
		t.Errorf("String: got %q, want %q", tr.String(), "true")
	}

	if fa.String() != "false" {
		t.Errorf("String: got %q, want %q", fa.String(), "false")
	}

	if tr.Equals(fa) {
		t.Error("true should not equal false")
	}

	if !tr.Equals(LiteralBool(true)) {
		t.Error("true should equal true")
	}
}

func TestLiteralInt(t *testing.T) {
	lit := LiteralInt(42)

	if lit.Kind() != kind.Literal {
		t.Errorf("Kind: got %v, want Literal", lit.Kind())
	}

	if lit.Base != kind.Integer {
		t.Errorf("Base: got %v, want Integer", lit.Base)
	}

	if lit.Value != int64(42) {
		t.Errorf("Value: got %v, want 42", lit.Value)
	}

	if lit.String() != "42" {
		t.Errorf("String: got %q, want %q", lit.String(), "42")
	}

	neg := LiteralInt(-100)
	if neg.String() != "-100" {
		t.Errorf("String: got %q, want %q", neg.String(), "-100")
	}

	if lit.Equals(neg) {
		t.Error("42 should not equal -100")
	}

	if !lit.Equals(LiteralInt(42)) {
		t.Error("42 should equal 42")
	}
}

func TestLiteralNumber(t *testing.T) {
	lit := LiteralNumber(3.14)

	if lit.Kind() != kind.Literal {
		t.Errorf("Kind: got %v, want Literal", lit.Kind())
	}

	if lit.Base != kind.Number {
		t.Errorf("Base: got %v, want Number", lit.Base)
	}

	if lit.Value != 3.14 {
		t.Errorf("Value: got %v, want 3.14", lit.Value)
	}

	zero := LiteralNumber(0.0)
	if lit.Equals(zero) {
		t.Error("3.14 should not equal 0.0")
	}

	if !lit.Equals(LiteralNumber(3.14)) {
		t.Error("3.14 should equal 3.14")
	}
}

func TestLiteralString(t *testing.T) {
	lit := LiteralString("hello")

	if lit.Kind() != kind.Literal {
		t.Errorf("Kind: got %v, want Literal", lit.Kind())
	}

	if lit.Base != kind.String {
		t.Errorf("Base: got %v, want String", lit.Base)
	}

	if lit.Value != "hello" {
		t.Errorf("Value: got %v, want hello", lit.Value)
	}

	if lit.String() != `"hello"` {
		t.Errorf("String: got %q, want %q", lit.String(), `"hello"`)
	}

	other := LiteralString("world")
	if lit.Equals(other) {
		t.Error("hello should not equal world")
	}

	if !lit.Equals(LiteralString("hello")) {
		t.Error("hello should equal hello")
	}

	empty := LiteralString("")
	if empty.String() != `""` {
		t.Errorf("String: got %q, want %q", empty.String(), `""`)
	}
}

func TestLiteralSingletons(t *testing.T) {
	if True.Value != true {
		t.Error("True should have value true")
	}

	if False.Value != false {
		t.Error("False should have value false")
	}

	if !True.Equals(LiteralBool(true)) {
		t.Error("True should equal LiteralBool(true)")
	}

	if !False.Equals(LiteralBool(false)) {
		t.Error("False should equal LiteralBool(false)")
	}
}

func TestLiteralHashUniqueness(t *testing.T) {
	literals := []Type{
		LiteralBool(true),
		LiteralBool(false),
		LiteralInt(0),
		LiteralInt(1),
		LiteralInt(-1),
		LiteralNumber(0.0),
		LiteralNumber(1.0),
		LiteralString(""),
		LiteralString("a"),
		LiteralString("b"),
	}

	hashes := make(map[uint64]Type)

	for _, lit := range literals {
		h := lit.Hash()
		if existing, ok := hashes[h]; ok {
			t.Errorf("Hash collision: %s and %s both have hash %d",
				existing.String(), lit.String(), h)
		}

		hashes[h] = lit
	}
}

func TestLiteralNotEqualToPrimitive(t *testing.T) {
	if LiteralBool(true).Equals(Boolean) {
		t.Error("LiteralBool(true) should not equal Boolean")
	}

	if LiteralInt(42).Equals(Integer) {
		t.Error("LiteralInt(42) should not equal Integer")
	}

	if LiteralNumber(3.14).Equals(Number) {
		t.Error("LiteralNumber(3.14) should not equal Number")
	}

	if LiteralString("x").Equals(String) {
		t.Error("LiteralString(x) should not equal String")
	}
}

func TestLiteralBoolInterning(t *testing.T) {
	t1 := LiteralBool(true)
	t2 := LiteralBool(true)
	f1 := LiteralBool(false)
	f2 := LiteralBool(false)

	if t1 != True {
		t.Error("LiteralBool(true) should return typ.True singleton")
	}

	if t2 != True {
		t.Error("LiteralBool(true) should return typ.True singleton")
	}

	if f1 != False {
		t.Error("LiteralBool(false) should return typ.False singleton")
	}

	if f2 != False {
		t.Error("LiteralBool(false) should return typ.False singleton")
	}

	if t1 != t2 {
		t.Error("multiple LiteralBool(true) calls should return same pointer")
	}

	if f1 != f2 {
		t.Error("multiple LiteralBool(false) calls should return same pointer")
	}

	if t1.Hash() != True.Hash() {
		t.Errorf("hash mismatch: LiteralBool(true)=%d, True=%d", t1.Hash(), True.Hash())
	}

	if f1.Hash() != False.Hash() {
		t.Errorf("hash mismatch: LiteralBool(false)=%d, False=%d", f1.Hash(), False.Hash())
	}
}
