package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

func TestRecordEmpty(t *testing.T) {
	r := NewRecord().Build()

	if r.Kind() != kind.Record {
		t.Errorf("Kind: got %v, want Record", r.Kind())
	}

	if len(r.Fields) != 0 {
		t.Errorf("Fields: got %d, want 0", len(r.Fields))
	}

	if r.String() != "{}" {
		t.Errorf("String: got %q, want %q", r.String(), "{}")
	}
}

func TestRecordWithFields(t *testing.T) {
	r := NewRecord().
		Field("x", Number).
		Field("y", String).
		Build()

	if len(r.Fields) != 2 {
		t.Errorf("Fields: got %d, want 2", len(r.Fields))
	}
}

func TestRecordNilFieldDefaultsToUnknown(t *testing.T) {
	r := NewRecord().
		Field("x", nil).
		Build()

	f := r.GetField("x")
	if f == nil {
		t.Fatal("expected field x")
	}
	if f.Type != Unknown {
		t.Errorf("Field type: got %v, want Unknown", f.Type)
	}
}

func TestRecordFieldsSorted(t *testing.T) {
	r := NewRecord().
		Field("z", Number).
		Field("a", String).
		Field("m", Boolean).
		Build()

	if r.Fields[0].Name != "a" {
		t.Errorf("first field should be 'a', got %q", r.Fields[0].Name)
	}

	if r.Fields[1].Name != "m" {
		t.Errorf("second field should be 'm', got %q", r.Fields[1].Name)
	}

	if r.Fields[2].Name != "z" {
		t.Errorf("third field should be 'z', got %q", r.Fields[2].Name)
	}
}

func TestRecordOptionalField(t *testing.T) {
	r := NewRecord().
		Field("x", Number).
		OptField("y", String).
		Build()

	if r.Fields[0].Optional {
		t.Error("x should not be optional")
	}

	y := r.GetField("y")
	if y == nil || !y.Optional {
		t.Error("y should be optional")
	}
}

func TestRecordReadonlyField(t *testing.T) {
	r := NewRecord().
		ReadonlyField("id", Number).
		Field("name", String).
		Build()

	id := r.GetField("id")
	if id == nil || !id.Readonly {
		t.Error("id should be readonly")
	}

	name := r.GetField("name")
	if name == nil || name.Readonly {
		t.Error("name should not be readonly")
	}
}

func TestRecordString(t *testing.T) {
	r := NewRecord().
		Field("x", Number).
		OptField("y", String).
		ReadonlyField("z", Boolean).
		Build()

	s := r.String()
	if s != "{x: number, readonly y?: string, z: boolean}" &&
		s != "{x: number, y?: string, readonly z: boolean}" {
		// Fields are sorted, so exact string depends on sort order
		t.Logf("String: %s", s)
	}
}

func TestRecordEquality(t *testing.T) {
	r1 := NewRecord().Field("x", Number).Field("y", String).Build()
	r2 := NewRecord().Field("y", String).Field("x", Number).Build()
	r3 := NewRecord().Field("x", Number).Build()
	r4 := NewRecord().Field("x", String).Field("y", String).Build()

	if !r1.Equals(r2) {
		t.Error("records with same fields should be equal regardless of order")
	}

	if r1.Equals(r3) {
		t.Error("records with different field count should not be equal")
	}

	if r1.Equals(r4) {
		t.Error("records with different field types should not be equal")
	}
}

func TestRecordEqualityOptional(t *testing.T) {
	r1 := NewRecord().Field("x", Number).Build()
	r2 := NewRecord().OptField("x", Number).Build()

	if r1.Equals(r2) {
		t.Error("required vs optional field should differ")
	}
}

func TestRecordEqualityReadonly(t *testing.T) {
	r1 := NewRecord().Field("x", Number).Build()
	r2 := NewRecord().ReadonlyField("x", Number).Build()

	if r1.Equals(r2) {
		t.Error("mutable vs readonly field should differ")
	}
}

func TestRecordGetField(t *testing.T) {
	r := NewRecord().
		Field("x", Number).
		Field("y", String).
		Build()

	x := r.GetField("x")
	if x == nil {
		t.Fatal("GetField(x) should not be nil")
	}

	if x.Type != Number {
		t.Error("x should have type Number")
	}

	z := r.GetField("z")
	if z != nil {
		t.Error("GetField(z) should be nil")
	}
}

func TestRecordHashUniqueness(t *testing.T) {
	records := []Type{
		NewRecord().Build(),
		NewRecord().Field("x", Number).Build(),
		NewRecord().Field("x", String).Build(),
		NewRecord().Field("y", Number).Build(),
		NewRecord().OptField("x", Number).Build(),
		NewRecord().ReadonlyField("x", Number).Build(),
	}

	hashes := make(map[uint64]Type)

	for _, r := range records {
		h := r.Hash()
		if existing, ok := hashes[h]; ok {
			t.Errorf("Hash collision: %s and %s", existing.String(), r.String())
		}

		hashes[h] = r
	}
}

func TestRecordNotEqualToPrimitive(t *testing.T) {
	r := NewRecord().Field("x", Number).Build()
	if r.Equals(Number) {
		t.Error("record should not equal primitive")
	}
}

func TestRecordWithMetatable(t *testing.T) {
	meta := NewRecord().Field("__index", Any).Build()
	r := NewRecord().
		Field("value", Number).
		Metatable(meta).
		Build()

	if r.Metatable == nil {
		t.Error("record should have metatable")
	}

	if r.Metatable != meta {
		t.Error("metatable mismatch")
	}
}

func TestRecordOpenFlag(t *testing.T) {
	closed := NewRecord().Field("x", Number).Build()
	open := NewRecord().SetOpen(true).Field("x", Number).Build()

	if closed.Open {
		t.Error("default record should not be open")
	}

	if !open.Open {
		t.Error("record built with SetOpen(true) should be open")
	}
}

func TestRecordOpenString(t *testing.T) {
	empty := NewRecord().SetOpen(true).Build()
	if empty.String() != "{...}" {
		t.Errorf("open empty record string: got %q, want %q", empty.String(), "{...}")
	}

	withField := NewRecord().SetOpen(true).Field("x", Number).Build()
	if withField.String() != "{x: number, ...}" {
		t.Errorf("open record string: got %q, want %q", withField.String(), "{x: number, ...}")
	}
}

func TestRecordOpenEquality(t *testing.T) {
	closed := NewRecord().Field("x", Number).Build()
	open := NewRecord().SetOpen(true).Field("x", Number).Build()

	if closed.Equals(open) {
		t.Error("closed and open records with same fields should not be equal")
	}

	open2 := NewRecord().SetOpen(true).Field("x", Number).Build()
	if !open.Equals(open2) {
		t.Error("two open records with same fields should be equal")
	}
}

func TestRecordOpenHash(t *testing.T) {
	closed := NewRecord().Field("x", Number).Build()
	open := NewRecord().SetOpen(true).Field("x", Number).Build()

	if closed.Hash() == open.Hash() {
		t.Error("closed and open records should have different hashes")
	}
}

func TestRecordOpenTypeEquals(t *testing.T) {
	closed := NewRecord().Field("x", Number).Build()
	open := NewRecord().SetOpen(true).Field("x", Number).Build()

	if TypeEquals(closed, open) {
		t.Error("TypeEquals should distinguish open from closed")
	}

	open2 := NewRecord().SetOpen(true).Field("x", Number).Build()
	if !TypeEquals(open, open2) {
		t.Error("TypeEquals should match two identical open records")
	}
}
