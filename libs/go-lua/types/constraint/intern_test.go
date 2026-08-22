package constraint

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
)

func TestInternerTruthy(t *testing.T) {
	interner := NewInterner()
	p := NewPath(cfg.SymbolID(1), "x")

	c1 := interner.Truthy(p)
	c2 := interner.Truthy(p)

	if !c1.Equals(c2) {
		t.Error("interned constraints should be equal")
	}

	// Same path key should return same constraint
	p2 := NewPath(cfg.SymbolID(1), "x")
	c3 := interner.Truthy(p2)
	if !c1.Equals(c3) {
		t.Error("same path key should return equal constraint")
	}
}

func TestInternerFalsy(t *testing.T) {
	interner := NewInterner()
	p := NewPath(cfg.SymbolID(1), "x")

	c1 := interner.Falsy(p)
	c2 := interner.Falsy(p)

	if !c1.Equals(c2) {
		t.Error("interned constraints should be equal")
	}
}

func TestInternerIsNil(t *testing.T) {
	interner := NewInterner()
	p := NewPath(cfg.SymbolID(1), "x")

	c1 := interner.IsNil(p)
	c2 := interner.IsNil(p)

	if !c1.Equals(c2) {
		t.Error("interned constraints should be equal")
	}
}

func TestInternerNotNil(t *testing.T) {
	interner := NewInterner()
	p := NewPath(cfg.SymbolID(1), "x")

	c1 := interner.NotNil(p)
	c2 := interner.NotNil(p)

	if !c1.Equals(c2) {
		t.Error("interned constraints should be equal")
	}
}

func TestInternerSize(t *testing.T) {
	interner := NewInterner()
	p1 := NewPath(cfg.SymbolID(1), "x")
	p2 := NewPath(cfg.SymbolID(2), "y")

	if interner.Size() != 0 {
		t.Errorf("expected size 0, got %d", interner.Size())
	}

	interner.Truthy(p1)
	interner.Falsy(p1)
	interner.IsNil(p2)
	interner.NotNil(p2)

	if interner.Size() != 4 {
		t.Errorf("expected size 4, got %d", interner.Size())
	}

	// Same paths should not increase size
	interner.Truthy(p1)
	interner.Truthy(p1)

	if interner.Size() != 4 {
		t.Errorf("expected size 4 after duplicates, got %d", interner.Size())
	}
}

func TestInternerClear(t *testing.T) {
	interner := NewInterner()
	p := NewPath(cfg.SymbolID(1), "x")

	interner.Truthy(p)
	interner.Falsy(p)

	if interner.Size() == 0 {
		t.Error("expected non-zero size before clear")
	}

	interner.Clear()

	if interner.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", interner.Size())
	}
}

func TestInternerEmptyPath(t *testing.T) {
	interner := NewInterner()
	emptyPath := Path{}

	c := interner.Truthy(emptyPath)
	if !c.Path.IsEmpty() {
		t.Error("empty path should return constraint with empty path")
	}

	// Empty paths should not be stored
	if interner.Size() != 0 {
		t.Errorf("empty paths should not be stored, got size %d", interner.Size())
	}
}

func TestInternerConcurrency(t *testing.T) {
	interner := NewInterner()
	paths := make([]Path, 100)
	for i := range paths {
		paths[i] = NewPath(cfg.SymbolID(i+1), "v")
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, p := range paths {
				interner.Truthy(p)
				interner.Falsy(p)
				interner.IsNil(p)
				interner.NotNil(p)
			}
		}()
	}
	wg.Wait()

	// Should have exactly 400 entries (100 paths x 4 constraint types)
	if interner.Size() != 400 {
		t.Errorf("expected size 400, got %d", interner.Size())
	}
}

func BenchmarkInternerTruthy(b *testing.B) {
	interner := NewInterner()
	paths := make([]Path, 100)
	for i := range paths {
		paths[i] = NewPath(cfg.SymbolID(i+1), "v")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := paths[i%100]
		_ = interner.Truthy(p)
	}
}

func BenchmarkInternerTruthyParallel(b *testing.B) {
	interner := NewInterner()
	paths := make([]Path, 100)
	for i := range paths {
		paths[i] = NewPath(cfg.SymbolID(i+1), "v")
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			p := paths[i%100]
			_ = interner.Truthy(p)
			i++
		}
	})
}

func BenchmarkConstraintWithoutInterning(b *testing.B) {
	paths := make([]Path, 100)
	for i := range paths {
		paths[i] = NewPath(cfg.SymbolID(i+1), "v")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := paths[i%100]
		_ = Truthy{Path: p}
	}
}
