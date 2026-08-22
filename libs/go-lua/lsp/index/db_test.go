package index

import (
	"sync"
	"testing"
)

func TestDB_GetSet(t *testing.T) {
	db := NewDB()
	k := Key{File: "test.lua", Func: "foo", Kind: "check"}

	// Initially empty
	if _, ok := db.Get(k); ok {
		t.Error("expected cache miss for new key")
	}

	// Set value
	db.Set(k, "result", nil)

	// Get value
	v, ok := db.Get(k)
	if !ok {
		t.Error("expected cache hit after set")
	}
	if v != "result" {
		t.Errorf("expected 'result', got %v", v)
	}
}

func TestDB_Query(t *testing.T) {
	db := NewDB()
	k := Key{File: "test.lua", Func: "bar", Kind: "infer"}

	computed := 0
	result := db.Query(k, func() any {
		computed++
		return "computed"
	})

	if computed != 1 {
		t.Errorf("expected compute called once, got %d", computed)
	}
	if result != "computed" {
		t.Errorf("expected 'computed', got %v", result)
	}

	// Second query should use cache
	result2 := db.Query(k, func() any {
		computed++
		return "computed again"
	})

	if computed != 1 {
		t.Errorf("expected compute not called on cache hit, got %d", computed)
	}
	if result2 != "computed" {
		t.Errorf("expected cached 'computed', got %v", result2)
	}
}

func TestDB_InvalidateFile(t *testing.T) {
	db := NewDB()
	k1 := Key{File: "a.lua", Func: "foo", Kind: "check"}
	k2 := Key{File: "a.lua", Func: "bar", Kind: "check"}
	k3 := Key{File: "b.lua", Func: "baz", Kind: "check"}

	db.Set(k1, "v1", nil)
	db.Set(k2, "v2", nil)
	db.Set(k3, "v3", nil)

	// Invalidate file a.lua
	db.InvalidateFile("a.lua")

	if _, ok := db.Get(k1); ok {
		t.Error("k1 should be invalidated")
	}
	if _, ok := db.Get(k2); ok {
		t.Error("k2 should be invalidated")
	}
	if _, ok := db.Get(k3); !ok {
		t.Error("k3 should still be valid")
	}
}

func TestDB_InvalidateFunc(t *testing.T) {
	db := NewDB()
	k1 := Key{File: "a.lua", Func: "foo", Kind: "check"}
	k2 := Key{File: "a.lua", Func: "foo", Kind: "infer"}
	k3 := Key{File: "a.lua", Func: "bar", Kind: "check"}

	db.Set(k1, "v1", nil)
	db.Set(k2, "v2", nil)
	db.Set(k3, "v3", nil)

	// Invalidate only foo
	db.InvalidateFunc("a.lua", "foo")

	if _, ok := db.Get(k1); ok {
		t.Error("k1 should be invalidated")
	}
	if _, ok := db.Get(k2); ok {
		t.Error("k2 should be invalidated")
	}
	if _, ok := db.Get(k3); !ok {
		t.Error("k3 should still be valid")
	}
}

func TestDB_Clear(t *testing.T) {
	db := NewDB()
	k := Key{File: "test.lua", Func: "foo", Kind: "check"}

	db.Set(k, "value", nil)
	if db.Size() != 1 {
		t.Errorf("expected size 1, got %d", db.Size())
	}

	db.Clear()

	if db.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", db.Size())
	}
	if _, ok := db.Get(k); ok {
		t.Error("expected cache miss after clear")
	}
}

func TestDB_Version(t *testing.T) {
	db := NewDB()

	v0 := db.Version()

	// File invalidation does not bump version
	db.InvalidateFile("test.lua")
	v1 := db.Version()

	if v1 != v0 {
		t.Error("version should not change after file invalidation")
	}

	// Clear bumps version
	db.Clear()
	v2 := db.Version()

	if v2 <= v1 {
		t.Error("version should increase after clear")
	}
}

func TestDB_Keys(t *testing.T) {
	db := NewDB()
	k1 := Key{File: "a.lua", Func: "foo", Kind: "check"}
	k2 := Key{File: "b.lua", Func: "bar", Kind: "infer"}

	db.Set(k1, "v1", nil)
	db.Set(k2, "v2", nil)

	keys := db.Keys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestDB_KeysForFile(t *testing.T) {
	db := NewDB()
	k1 := Key{File: "a.lua", Func: "foo", Kind: "check"}
	k2 := Key{File: "a.lua", Func: "bar", Kind: "infer"}
	k3 := Key{File: "b.lua", Func: "baz", Kind: "check"}

	db.Set(k1, "v1", nil)
	db.Set(k2, "v2", nil)
	db.Set(k3, "v3", nil)

	keys := db.KeysForFile("a.lua")
	if len(keys) != 2 {
		t.Errorf("expected 2 keys for a.lua, got %d", len(keys))
	}
}

func TestDB_Has(t *testing.T) {
	db := NewDB()
	k := Key{File: "test.lua", Func: "foo", Kind: "check"}

	if db.Has(k) {
		t.Error("expected Has to return false for missing key")
	}

	db.Set(k, "value", nil)

	if !db.Has(k) {
		t.Error("expected Has to return true after set")
	}
}

func TestDB_Delete(t *testing.T) {
	db := NewDB()
	k := Key{File: "test.lua", Func: "foo", Kind: "check"}

	db.Set(k, "value", nil)
	if !db.Has(k) {
		t.Error("expected key to exist")
	}

	db.Delete(k)
	if db.Has(k) {
		t.Error("expected key to be deleted")
	}
}

func TestDB_Concurrent(t *testing.T) {
	db := NewDB()
	var wg sync.WaitGroup

	// Concurrent reads and writes
	for i := 0; i < 100; i++ {
		wg.Add(2)

		go func(i int) {
			defer wg.Done()
			k := Key{File: "test.lua", Func: "foo", Kind: "check"}
			db.Set(k, i, nil)
		}(i)

		go func() {
			defer wg.Done()
			k := Key{File: "test.lua", Func: "foo", Kind: "check"}
			db.Get(k)
		}()
	}

	wg.Wait()
}

func TestDB_QueryWithDeps(t *testing.T) {
	db := NewDB()
	k := Key{File: "test.lua", Func: "foo", Kind: "check"}
	deps := []Key{
		{File: "util.lua", Func: "", Kind: "types"},
	}

	db.QueryWithDeps(k, deps, func() any {
		return "value"
	})

	if !db.Has(k) {
		t.Error("expected key to be cached")
	}

	// Second call should return cached value
	computed := 0
	result := db.QueryWithDeps(k, deps, func() any {
		computed++
		return "new value"
	})
	if computed != 0 {
		t.Error("expected cache hit, compute should not be called")
	}
	if result != "value" {
		t.Errorf("expected 'value', got %v", result)
	}
}

func TestDB_VersionInvalidation(t *testing.T) {
	db := NewDB()
	k := Key{File: "test.lua", Func: "foo", Kind: "check"}

	db.Set(k, "value", nil)
	if _, ok := db.Get(k); !ok {
		t.Error("expected cache hit")
	}

	// Invalidating a different file does not affect this entry
	db.InvalidateFile("other.lua")

	// Entry should still be valid
	if _, ok := db.Get(k); !ok {
		t.Error("expected cache hit - different file invalidation")
	}

	// Clear invalidates everything via version bump
	db.Clear()
	db.Set(k, "value2", nil)
	v1 := db.Version()

	db.Clear()

	// Entry from before clear should be gone
	if _, ok := db.Get(k); ok {
		t.Error("expected cache miss after clear")
	}

	// New entries after clear work
	db.Set(k, "value3", nil)
	if _, ok := db.Get(k); !ok {
		t.Error("expected cache hit for new entry")
	}
	_ = v1
}

func TestDB_InvalidateWithDependents(t *testing.T) {
	db := NewDB()

	// Create a dependency chain: base -> derived1 -> derived2
	base := Key{File: "base.lua", Func: "base", Kind: "types"}
	derived1 := Key{File: "a.lua", Func: "foo", Kind: "check"}
	derived2 := Key{File: "b.lua", Func: "bar", Kind: "check"}
	unrelated := Key{File: "c.lua", Func: "baz", Kind: "check"}

	db.Set(base, "base_value", nil)
	db.Set(derived1, "derived1_value", []Key{base})
	db.Set(derived2, "derived2_value", []Key{derived1})
	db.Set(unrelated, "unrelated_value", nil)

	// Invalidate base - should cascade to derived1 and derived2
	db.InvalidateWithDependents(base)

	if db.Has(base) {
		t.Error("base should be invalidated")
	}
	if db.Has(derived1) {
		t.Error("derived1 should be invalidated (depends on base)")
	}
	if db.Has(derived2) {
		t.Error("derived2 should be invalidated (depends on derived1)")
	}
	if !db.Has(unrelated) {
		t.Error("unrelated should still exist")
	}
}

func TestDB_InvalidateWithDependents_NoDeps(t *testing.T) {
	db := NewDB()

	k := Key{File: "test.lua", Func: "foo", Kind: "check"}
	other := Key{File: "test.lua", Func: "bar", Kind: "check"}

	db.Set(k, "value", nil)
	db.Set(other, "other", nil)

	db.InvalidateWithDependents(k)

	if db.Has(k) {
		t.Error("k should be invalidated")
	}
	if !db.Has(other) {
		t.Error("other should still exist (no dependency)")
	}
}

func TestDB_InvalidateFileWithDependents(t *testing.T) {
	db := NewDB()

	// Entries in file a.lua
	a1 := Key{File: "a.lua", Func: "foo", Kind: "types"}
	a2 := Key{File: "a.lua", Func: "bar", Kind: "types"}

	// Entry in b.lua that depends on a.lua
	b1 := Key{File: "b.lua", Func: "baz", Kind: "check"}

	// Entry in c.lua that depends on b.lua
	c1 := Key{File: "c.lua", Func: "qux", Kind: "check"}

	// Independent entry
	d1 := Key{File: "d.lua", Func: "independent", Kind: "check"}

	db.Set(a1, "a1_value", nil)
	db.Set(a2, "a2_value", nil)
	db.Set(b1, "b1_value", []Key{a1})
	db.Set(c1, "c1_value", []Key{b1})
	db.Set(d1, "d1_value", nil)

	// Invalidate all entries from a.lua with cascade
	db.InvalidateFileWithDependents("a.lua")

	if db.Has(a1) {
		t.Error("a1 should be invalidated")
	}
	if db.Has(a2) {
		t.Error("a2 should be invalidated")
	}
	if db.Has(b1) {
		t.Error("b1 should be invalidated (depends on a1)")
	}
	if db.Has(c1) {
		t.Error("c1 should be invalidated (depends on b1)")
	}
	if !db.Has(d1) {
		t.Error("d1 should still exist (independent)")
	}
}

func TestDB_InvalidateFileWithDependents_EmptyFile(t *testing.T) {
	db := NewDB()

	k := Key{File: "test.lua", Func: "foo", Kind: "check"}
	db.Set(k, "value", nil)

	// Invalidate non-existent file
	db.InvalidateFileWithDependents("nonexistent.lua")

	if !db.Has(k) {
		t.Error("k should still exist")
	}
}

func TestDB_InvalidateCascade_CyclicDeps(t *testing.T) {
	db := NewDB()

	// Create a cycle: a -> b -> c -> a
	a := Key{File: "a.lua", Func: "a", Kind: "check"}
	b := Key{File: "b.lua", Func: "b", Kind: "check"}
	c := Key{File: "c.lua", Func: "c", Kind: "check"}

	db.Set(a, "a_value", []Key{c})
	db.Set(b, "b_value", []Key{a})
	db.Set(c, "c_value", []Key{b})

	// Should not infinite loop
	db.InvalidateWithDependents(a)

	if db.Has(a) || db.Has(b) || db.Has(c) {
		t.Error("all entries in cycle should be invalidated")
	}
}
