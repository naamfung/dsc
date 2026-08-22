package db

import "testing"

func TestAttachmentKey_Name(t *testing.T) {
	key := NewAttachmentKey[string]("test.key")
	if key.Name() != "test.key" {
		t.Errorf("Name() = %q, want %q", key.Name(), "test.key")
	}
}

func TestAttach_Attached(t *testing.T) {
	ctx := NewQueryContext(New())
	key := NewAttachmentKey[string]("test.string")

	Attach(ctx, key, "hello")

	val, ok := Attached(ctx, key)
	if !ok {
		t.Error("Attached should return true for attached value")
	}
	if val != "hello" {
		t.Errorf("Attached = %q, want %q", val, "hello")
	}
}

func TestAttach_DifferentTypes(t *testing.T) {
	ctx := NewQueryContext(New())
	stringKey := NewAttachmentKey[string]("test.string")
	intKey := NewAttachmentKey[int]("test.int")

	Attach(ctx, stringKey, "hello")
	Attach(ctx, intKey, 42)

	strVal, ok := Attached(ctx, stringKey)
	if !ok || strVal != "hello" {
		t.Errorf("string attachment = %v, %v; want hello, true", strVal, ok)
	}

	intVal, ok := Attached(ctx, intKey)
	if !ok || intVal != 42 {
		t.Errorf("int attachment = %v, %v; want 42, true", intVal, ok)
	}
}

func TestAttached_MissingKey(t *testing.T) {
	ctx := NewQueryContext(New())
	key := NewAttachmentKey[string]("test.missing")

	val, ok := Attached(ctx, key)
	if ok {
		t.Error("Attached should return false for missing key")
	}
	if val != "" {
		t.Errorf("Attached = %q, want empty string (zero value)", val)
	}
}

func TestAttached_NilContext(t *testing.T) {
	key := NewAttachmentKey[string]("test.key")

	val, ok := Attached(nil, key)
	if ok {
		t.Error("Attached(nil, key) should return false")
	}
	if val != "" {
		t.Errorf("Attached(nil, key) = %q, want empty string", val)
	}
}

func TestAttach_NilContext(_ *testing.T) {
	key := NewAttachmentKey[string]("test.key")
	Attach(nil, key, "value") // should not panic
}

func TestAttach_PointerType(t *testing.T) {
	type TestStruct struct {
		Value int
	}

	ctx := NewQueryContext(New())
	key := NewAttachmentKey[*TestStruct]("test.struct")
	original := &TestStruct{Value: 100}

	Attach(ctx, key, original)

	retrieved, ok := Attached(ctx, key)
	if !ok {
		t.Error("Attached should return true for pointer attachment")
	}
	if retrieved != original {
		t.Error("Attached should return the same pointer")
	}
	if retrieved.Value != 100 {
		t.Errorf("retrieved.Value = %d, want 100", retrieved.Value)
	}
}

func TestAttach_Overwrite(t *testing.T) {
	ctx := NewQueryContext(New())
	key := NewAttachmentKey[string]("test.overwrite")

	Attach(ctx, key, "first")
	Attach(ctx, key, "second")

	val, ok := Attached(ctx, key)
	if !ok || val != "second" {
		t.Errorf("Attached after overwrite = %q, %v; want second, true", val, ok)
	}
}

func TestAttach_NilValue(t *testing.T) {
	type TestStruct struct {
		Value int
	}

	ctx := NewQueryContext(New())
	key := NewAttachmentKey[*TestStruct]("test.nil")

	Attach(ctx, key, nil)

	val, ok := Attached(ctx, key)
	if !ok {
		t.Error("Attached should return true for nil value")
	}
	if val != nil {
		t.Error("Attached should return nil value")
	}
}
