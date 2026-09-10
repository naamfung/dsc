package lua

import (
	"testing"
)

type testRegistryHandler struct {
	overflowCalled bool
}

func (h *testRegistryHandler) registryOverflow() {
	h.overflowCalled = true
}

func TestRegistryBasicOperations(t *testing.T) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 10, 5, 100)

	// Test initial state
	if rg.Top() != 0 {
		t.Errorf("initial top: expected 0, got %d", rg.Top())
	}

	// Test Push
	rg.Push(LNumber(1))
	rg.Push(LNumber(2))
	rg.Push(LNumber(3))

	if rg.Top() != 3 {
		t.Errorf("after push: expected top 3, got %d", rg.Top())
	}

	// Test Get
	if rg.Get(0).(LNumber) != 1 {
		t.Errorf("Get(0): expected 1, got %v", rg.Get(0))
	}
	if rg.Get(1).(LNumber) != 2 {
		t.Errorf("Get(1): expected 2, got %v", rg.Get(1))
	}

	// Test Pop
	v := rg.Pop()
	if v.(LNumber) != 3 {
		t.Errorf("Pop: expected 3, got %v", v)
	}
	if rg.Top() != 2 {
		t.Errorf("after pop: expected top 2, got %d", rg.Top())
	}
}

func TestRegistrySetTop(t *testing.T) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 10, 5, 100)

	// Push some values
	rg.Push(LNumber(1))
	rg.Push(LNumber(2))
	rg.Push(LNumber(3))

	// Increase top - should fill with LNil
	rg.SetTop(5)
	if rg.Top() != 5 {
		t.Errorf("SetTop(5): expected top 5, got %d", rg.Top())
	}
	if rg.Get(3) != LNil {
		t.Errorf("Get(3): expected LNil, got %v", rg.Get(3))
	}
	if rg.Get(4) != LNil {
		t.Errorf("Get(4): expected LNil, got %v", rg.Get(4))
	}

	// Decrease top
	rg.SetTop(2)
	if rg.Top() != 2 {
		t.Errorf("SetTop(2): expected top 2, got %d", rg.Top())
	}
}

func TestRegistrySet(t *testing.T) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 10, 5, 100)

	// Set beyond current top
	rg.Set(5, LNumber(100))
	if rg.Top() != 6 {
		t.Errorf("after Set(5): expected top 6, got %d", rg.Top())
	}
	if rg.Get(5).(LNumber) != 100 {
		t.Errorf("Get(5): expected 100, got %v", rg.Get(5))
	}

	// Set within current range
	rg.Set(2, LNumber(50))
	if rg.Get(2).(LNumber) != 50 {
		t.Errorf("Get(2): expected 50, got %v", rg.Get(2))
	}
}

func TestRegistryResize(t *testing.T) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 2, 2, 100) // small initial size

	// Push beyond initial capacity
	for i := 0; i < 10; i++ {
		rg.Push(LNumber(i))
	}

	if rg.Top() != 10 {
		t.Errorf("after 10 pushes: expected top 10, got %d", rg.Top())
	}

	// Verify all values
	for i := 0; i < 10; i++ {
		if rg.Get(i).(LNumber) != LNumber(i) {
			t.Errorf("Get(%d): expected %d, got %v", i, i, rg.Get(i))
		}
	}
}

func TestRegistryOverflow(t *testing.T) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 2, 2, 5) // small max size

	// Push until we hit max size
	for i := 0; i < 5; i++ {
		rg.Push(LNumber(i))
	}

	if handler.overflowCalled {
		t.Error("overflow should not be called yet")
	}

	// Next push should trigger overflow detection via resize
	// resize returns false but Push doesn't check it (design issue)
	// For now just verify the handler was called
	rg.resize(6)

	if !handler.overflowCalled {
		t.Error("expected overflow handler to be called")
	}
}

func TestRegistryCopyRange(t *testing.T) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 20, 5, 100)

	// Setup: push 5 values
	for i := 0; i < 5; i++ {
		rg.Push(LNumber(i))
	}

	// Copy range [1,3] to [10,12]
	rg.CopyRange(10, 1, -1, 3)

	if rg.Top() != 13 {
		t.Errorf("after CopyRange: expected top 13, got %d", rg.Top())
	}

	// Verify copied values
	if rg.Get(10).(LNumber) != 1 {
		t.Errorf("Get(10): expected 1, got %v", rg.Get(10))
	}
	if rg.Get(11).(LNumber) != 2 {
		t.Errorf("Get(11): expected 2, got %v", rg.Get(11))
	}
	if rg.Get(12).(LNumber) != 3 {
		t.Errorf("Get(12): expected 3, got %v", rg.Get(12))
	}
}

func TestRegistryFillNil(t *testing.T) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 20, 5, 100)

	// Setup: push some values
	rg.Push(LNumber(1))
	rg.Push(LNumber(2))
	rg.Push(LNumber(3))

	// Fill with nil starting at position 5
	rg.FillNil(5, 3)

	if rg.Top() != 8 {
		t.Errorf("after FillNil: expected top 8, got %d", rg.Top())
	}

	for i := 5; i < 8; i++ {
		if rg.Get(i) != LNil {
			t.Errorf("Get(%d): expected LNil, got %v", i, rg.Get(i))
		}
	}
}

func TestRegistryInsert(t *testing.T) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 20, 5, 100)

	// Push initial values
	rg.Push(LNumber(1))
	rg.Push(LNumber(2))
	rg.Push(LNumber(3))

	// Insert at beginning
	rg.Insert(LNumber(0), 0)

	if rg.Top() != 4 {
		t.Errorf("after Insert: expected top 4, got %d", rg.Top())
	}

	if rg.Get(0).(LNumber) != 0 {
		t.Errorf("Get(0): expected 0, got %v", rg.Get(0))
	}
	if rg.Get(1).(LNumber) != 1 {
		t.Errorf("Get(1): expected 1, got %v", rg.Get(1))
	}
}

func TestRegistryIsFull(t *testing.T) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 3, 2, 100)

	if rg.IsFull() {
		t.Error("registry should not be full initially")
	}

	rg.Push(LNumber(1))
	rg.Push(LNumber(2))
	rg.Push(LNumber(3))

	if !rg.IsFull() {
		t.Error("registry should be full after 3 pushes with capacity 3")
	}

	// After resize it shouldn't be full
	rg.Push(LNumber(4))
	if rg.IsFull() {
		t.Error("registry should not be full after resize")
	}
}

func TestRegistrySetNumber(t *testing.T) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 10, 5, 100)

	// SetNumber uses number boxing helper for non-preloaded values.
	rg.SetNumber(0, LNumber(1000)) // outside preload range

	v := rg.Get(0)
	if v.Type() != LTNumber {
		t.Errorf("expected LTNumber, got %v", v.Type())
	}
	if float64(v.(LNumber)) != 1000 {
		t.Errorf("expected 1000, got %v", v)
	}
}

func TestRegistryGetNilValue(t *testing.T) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 10, 5, 100)

	rg.SetTop(5)

	// Get should return LNil for uninitialized slots
	for i := 0; i < 5; i++ {
		if rg.Get(i) != LNil {
			t.Errorf("Get(%d): expected LNil, got %v", i, rg.Get(i))
		}
	}
}

func BenchmarkRegistryPush(b *testing.B) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 1024, 256, 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rg.Push(LNumber(i))
		if rg.Top() > 5000 {
			rg.SetTop(0)
		}
	}
}

func BenchmarkRegistrySetNumber(b *testing.B) {
	handler := &testRegistryHandler{}
	rg := newRegistry(handler, 1024, 256, 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rg.SetNumber(i%1000, LNumber(i))
	}
}
