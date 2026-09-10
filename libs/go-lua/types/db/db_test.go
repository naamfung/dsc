package db

import (
	"fmt"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRevisionStartsAtOne(t *testing.T) {
	db := New()
	if db.Revision() != 1 {
		t.Errorf("got %d, want 1", db.Revision())
	}
}

func TestBumpIncrements(t *testing.T) {
	db := New()
	r1 := db.Revision()
	r2 := db.Bump()

	if r2 != r1+1 {
		t.Errorf("got %d, want %d", r2, r1+1)
	}

	if db.Revision() != r2 {
		t.Errorf("Revision() should return bumped value")
	}
}

func TestInternReturnsCanonical(t *testing.T) {
	db := New()

	type testStruct struct{ value int }

	equal := func(existing, candidate any) bool {
		return existing.(*testStruct).value == candidate.(*testStruct).value
	}

	// Same key should return same instance
	v1 := db.Intern(123, func() any { return &testStruct{42} }, equal)
	v2 := db.Intern(123, func() any { return &testStruct{42} }, equal)

	if v1 != v2 {
		t.Error("same key should return same pointer")
	}

	if v1.(*testStruct).value != 42 {
		t.Error("should return first interned value")
	}
}

func TestInternDifferentKeys(t *testing.T) {
	db := New()

	equal := func(existing, candidate any) bool {
		return existing == candidate
	}

	v1 := db.Intern(1, func() any { return "a" }, equal)
	v2 := db.Intern(2, func() any { return "b" }, equal)

	if v1 == v2 {
		t.Error("different keys should return different values")
	}
}

func TestInternTypeCanonicalizes(t *testing.T) {
	db := New()

	t1 := typ.NewUnion(typ.String, typ.Number)
	t2 := typ.NewUnion(typ.String, typ.Number)

	c1 := db.InternType(t1)
	c2 := db.InternType(t2)

	if c1 != c2 {
		t.Fatal("InternType should return the same canonical pointer for equal types")
	}

	if !typ.TypeEquals(c1, c2) {
		t.Fatal("canonicalized types should remain equal")
	}
}

func TestInternConcurrent(t *testing.T) {
	db := New()

	var wg sync.WaitGroup

	results := make(chan any, 100)
	equal := func(existing, candidate any) bool {
		return existing == candidate
	}

	for range 100 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			v := db.Intern(42, func() any { return "shared" }, equal)
			results <- v
		}()
	}

	wg.Wait()
	close(results)

	// All results should be the same pointer
	var first any
	for v := range results {
		if first == nil {
			first = v
		} else if v != first {
			t.Error("concurrent intern should return same pointer")
		}
	}
}

func TestManifestOperations(t *testing.T) {
	t.Run("Connect and Manifest", func(t *testing.T) {
		db := New()
		rev := db.Revision()

		manifest := io.NewManifest("/test/path")
		db.Connect("/test/path", manifest)

		if db.Revision() <= rev {
			t.Error("Connect should bump revision")
		}

		m := db.Manifest("/test/path")
		if m != manifest {
			t.Errorf("got %v, want manifest1", m)
		}
	})

	t.Run("Manifest not found", func(t *testing.T) {
		db := New()

		m := db.Manifest("/nonexistent")
		if m != nil {
			t.Errorf("got %v, want nil", m)
		}
	})

	t.Run("Disconnect", func(t *testing.T) {
		db := New()
		manifest := io.NewManifest("/test/path")
		db.Connect("/test/path", manifest)
		rev := db.Revision()

		db.Disconnect("/test/path")

		if db.Revision() <= rev {
			t.Error("Disconnect should bump revision")
		}

		m := db.Manifest("/test/path")
		if m != nil {
			t.Errorf("got %v, want nil after disconnect", m)
		}
	})

	t.Run("Disconnect nonexistent", func(t *testing.T) {
		db := New()
		rev := db.Revision()
		db.Disconnect("/nonexistent")

		if db.Revision() <= rev {
			t.Error("Disconnect should bump revision even for nonexistent")
		}
	})

	t.Run("Overwrite manifest", func(t *testing.T) {
		db := New()
		first := io.NewManifest("/path")
		second := io.NewManifest("/path")

		db.Connect("/path", first)
		db.Connect("/path", second)

		m := db.Manifest("/path")
		if m != second {
			t.Errorf("got %v, want second", m)
		}
	})
}

func TestManifests(t *testing.T) {
	t.Run("iterate all", func(t *testing.T) {
		db := New()
		ma := io.NewManifest("/a")
		mb := io.NewManifest("/b")
		mc := io.NewManifest("/c")

		db.Connect("/a", ma)
		db.Connect("/b", mb)
		db.Connect("/c", mc)

		visited := make(map[string]*io.Manifest)

		db.Manifests(func(path string, manifest *io.Manifest) bool {
			visited[path] = manifest
			return true
		})

		if len(visited) != 3 {
			t.Errorf("got %d manifests, want 3", len(visited))
		}

		if visited["/a"] != ma || visited["/b"] != mb || visited["/c"] != mc {
			t.Error("manifests mismatch")
		}
	})

	t.Run("early termination", func(t *testing.T) {
		db := New()
		db.Connect("/a", io.NewManifest("/a"))
		db.Connect("/b", io.NewManifest("/b"))
		db.Connect("/c", io.NewManifest("/c"))

		count := 0

		db.Manifests(func(_ string, _ *io.Manifest) bool {
			count++
			return false
		})

		if count != 1 {
			t.Errorf("got %d iterations, want 1", count)
		}
	})

	t.Run("empty database", func(t *testing.T) {
		db := New()
		count := 0

		db.Manifests(func(_ string, _ *io.Manifest) bool {
			count++
			return true
		})

		if count != 0 {
			t.Errorf("got %d iterations, want 0", count)
		}
	})
}

func TestDBConcurrent(t *testing.T) {
	db := New()

	var wg sync.WaitGroup

	// Concurrent bumps
	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()
			db.Bump()
		}()
	}

	// Concurrent manifest operations
	for i := range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			path := fmt.Sprintf("/path/%d", i%10)
			db.Connect(path, io.NewManifest(path))
		}()
	}

	for i := range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			path := fmt.Sprintf("/path/%d", i%10)
			db.Manifest(path)
		}()
	}

	for range 10 {
		wg.Add(1)

		go func() {
			defer wg.Done()
			db.Manifests(func(_ string, _ *io.Manifest) bool {
				return true
			})
		}()
	}

	wg.Wait()

	// Verify final revision is sensible
	if db.Revision() < 51 {
		t.Errorf("revision should be at least 51, got %d", db.Revision())
	}
}

func TestInternNilValue(t *testing.T) {
	db := New()

	v := db.Intern(1, func() any { return nil }, nil)
	if v != nil {
		t.Errorf("got %v, want nil", v)
	}

	// Second call should return cached nil
	v2 := db.Intern(1, func() any { return "different" }, nil)
	if v2 != nil {
		t.Errorf("got %v, want nil (cached)", v2)
	}
}

func TestManifestNilValue(t *testing.T) {
	db := New()
	db.Connect("/path", nil)

	m := db.Manifest("/path")
	if m != nil {
		t.Errorf("got %v, want nil", m)
	}
}

func TestRevisionType(t *testing.T) {
	tests := []struct {
		name string
		r1   Revision
		r2   Revision
		want bool
	}{
		{"equal", 5, 5, true},
		{"less", 3, 5, true},
		{"greater", 7, 5, false},
		{"zero", 0, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r1 <= tt.r2
			if got != tt.want {
				t.Errorf("%d <= %d: got %v, want %v", tt.r1, tt.r2, got, tt.want)
			}
		})
	}
}

func TestInternerDirectAccess(t *testing.T) {
	i := newInterner()
	equal := func(existing, candidate any) bool {
		return existing == candidate
	}

	v1 := i.Intern(100, func() any { return "first" }, equal)
	v2 := i.Intern(100, func() any { return "first" }, equal)
	v3 := i.Intern(200, func() any { return "third" }, equal)

	if v1 != v2 {
		t.Error("same key should return same value")
	}

	if v1 == v3 {
		t.Error("different keys should return different values")
	}

	if v1 != "first" {
		t.Errorf("got %v, want first", v1)
	}

	if v3 != "third" {
		t.Errorf("got %v, want third", v3)
	}
}

func TestInternerConcurrentDifferentKeys(t *testing.T) {
	i := newInterner()

	var wg sync.WaitGroup

	equal := func(existing, candidate any) bool {
		return existing == candidate
	}

	for key := range uint64(100) {
		wg.Add(1)

		go func() {
			defer wg.Done()

			v := i.Intern(key, func() any { return key }, equal)
			if v != key {
				t.Errorf("key %d: got %v", key, v)
			}
		}()
	}

	wg.Wait()
}

func TestInternCollisionUsesEquality(t *testing.T) {
	db := New()

	type testStruct struct {
		label string
	}

	equal := func(existing, candidate any) bool {
		return existing.(*testStruct).label == candidate.(*testStruct).label
	}

	v1 := db.Intern(7, func() any { return &testStruct{label: "a"} }, equal)
	v2 := db.Intern(7, func() any { return &testStruct{label: "b"} }, equal)

	if v1 == v2 {
		t.Error("different values with same key should not be forced equal")
	}

	if v1.(*testStruct).label != "a" {
		t.Errorf("got %v, want a", v1.(*testStruct).label)
	}

	if v2.(*testStruct).label != "b" {
		t.Errorf("got %v, want b", v2.(*testStruct).label)
	}
}

func TestBumpMultiple(t *testing.T) {
	db := New()

	revisions := make([]Revision, 100)
	for i := range 100 {
		revisions[i] = db.Bump()
	}

	for i := 1; i < len(revisions); i++ {
		if revisions[i] != revisions[i-1]+1 {
			t.Errorf("revision %d: got %d, want %d", i, revisions[i], revisions[i-1]+1)
		}
	}

	if db.Revision() != 101 {
		t.Errorf("final revision: got %d, want 101", db.Revision())
	}
}

func TestEmptyPath(t *testing.T) {
	db := New()

	manifest := io.NewManifest("")
	db.Connect("", manifest)
	m := db.Manifest("")

	if m != manifest {
		t.Errorf("got %v, want manifest", m)
	}

	db.Disconnect("")
	m = db.Manifest("")

	if m != nil {
		t.Errorf("got %v, want nil", m)
	}
}

func TestDB_Imports(t *testing.T) {
	db := New()
	ma := io.NewManifest("/a")
	mb := io.NewManifest("/b")
	mc := io.NewManifest("/c")

	db.Connect("/a", ma)
	db.Connect("/b", mb)
	db.Connect("/c", mc)

	imports := db.Imports()

	if len(imports) != 3 {
		t.Errorf("got %d imports, want 3", len(imports))
	}
	if imports["/a"] != ma || imports["/b"] != mb || imports["/c"] != mc {
		t.Error("imports mismatch")
	}
}

func TestDB_Imports_Empty(t *testing.T) {
	db := New()
	imports := db.Imports()

	if imports == nil {
		t.Error("Imports should return non-nil map even when empty")
	}
	if len(imports) != 0 {
		t.Errorf("got %d imports, want 0", len(imports))
	}
}

func TestDB_Imports_Nil(t *testing.T) {
	var db *DB
	imports := db.Imports()

	if imports != nil {
		t.Errorf("nil DB.Imports should return nil, got %v", imports)
	}
}
