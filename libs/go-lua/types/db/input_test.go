package db

import "testing"

func TestInput_SetGet(t *testing.T) {
	db := New()
	input := NewInput[string, int](db)

	input.Set("a", 42)

	val, ok := input.Get(nil, "a")
	if !ok {
		t.Fatal("expected key 'a' to exist")
	}

	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}

func TestInput_GetMissing(t *testing.T) {
	db := New()
	input := NewInput[string, int](db)

	val, ok := input.Get(nil, "missing")
	if ok {
		t.Error("expected key to not exist")
	}

	if val != 0 {
		t.Errorf("expected zero value, got %d", val)
	}
}

func TestInput_Update(t *testing.T) {
	db := New()
	input := NewInput[string, int](db)

	input.Set("a", 10)
	input.Set("a", 20)

	val, ok := input.Get(nil, "a")
	if !ok {
		t.Fatal("expected key to exist")
	}

	if val != 20 {
		t.Errorf("expected 20, got %d", val)
	}
}

func TestInput_MultipleKeys(t *testing.T) {
	db := New()
	input := NewInput[string, string](db)

	input.Set("x", "foo")
	input.Set("y", "bar")
	input.Set("z", "baz")

	tests := []struct {
		key  string
		want string
	}{
		{"x", "foo"},
		{"y", "bar"},
		{"z", "baz"},
	}
	for _, tt := range tests {
		val, ok := input.Get(nil, tt.key)
		if !ok {
			t.Errorf("key %q should exist", tt.key)
		}

		if val != tt.want {
			t.Errorf("key %q: expected %q, got %q", tt.key, tt.want, val)
		}
	}
}

func TestInput_Range(t *testing.T) {
	db := New()
	input := NewInput[string, int](db)

	input.Set("a", 1)
	input.Set("b", 2)
	input.Set("c", 3)

	sum := 0
	count := 0

	input.Range(func(_ string, v int) bool {
		sum += v
		count++

		return true
	})

	if count != 3 {
		t.Errorf("expected 3 iterations, got %d", count)
	}

	if sum != 6 {
		t.Errorf("expected sum 6, got %d", sum)
	}
}

func TestInput_RangeEarlyTermination(t *testing.T) {
	db := New()
	input := NewInput[int, int](db)

	for i := range 10 {
		input.Set(i, i)
	}

	count := 0

	input.Range(func(_, _ int) bool {
		count++
		return count < 3
	})

	if count != 3 {
		t.Errorf("expected 3 iterations with early termination, got %d", count)
	}
}

func TestInput_NilReceiver(t *testing.T) {
	var input *Input[string, int]

	// Should not panic
	input.Set("a", 1)

	val, ok := input.Get(nil, "a")
	if ok {
		t.Error("nil input should return false")
	}

	if val != 0 {
		t.Error("nil input should return zero value")
	}

	// Range on nil should not panic
	input.Range(func(_ string, _ int) bool {
		t.Error("should not iterate on nil input")
		return true
	})
}

func TestInput_RevisionBumps(t *testing.T) {
	db := New()
	input := NewInput[string, int](db)

	r1 := db.Revision()

	input.Set("a", 1)

	r2 := db.Revision()

	if r2 <= r1 {
		t.Error("revision should increase after Set")
	}

	input.Set("a", 2)

	r3 := db.Revision()

	if r3 <= r2 {
		t.Error("revision should increase after second Set")
	}
}

func TestInput_WithContext(t *testing.T) {
	db := New()
	input := NewInput[string, int](db)

	input.Set("a", 42)

	ctx := NewQueryContext(db)

	val, ok := input.Get(ctx, "a")
	if !ok || val != 42 {
		t.Error("Get with context should return correct value")
	}
}

func TestInput_IntKey(t *testing.T) {
	db := New()
	input := NewInput[int, string](db)

	input.Set(1, "one")
	input.Set(2, "two")

	val, ok := input.Get(nil, 1)
	if !ok || val != "one" {
		t.Errorf("expected 'one', got %q, ok=%v", val, ok)
	}
}

func TestInput_StructValue(t *testing.T) {
	type Point struct {
		X, Y int
	}

	db := New()
	input := NewInput[string, Point](db)

	input.Set("origin", Point{0, 0})
	input.Set("p1", Point{1, 2})

	val, ok := input.Get(nil, "p1")
	if !ok {
		t.Fatal("expected key to exist")
	}

	if val.X != 1 || val.Y != 2 {
		t.Errorf("expected {1, 2}, got %+v", val)
	}
}
