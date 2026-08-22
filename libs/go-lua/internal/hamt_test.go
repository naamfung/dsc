package internal

import (
	"fmt"
	"testing"
)

// TestSetGet string constant to avoid goconst duplication.
const testValueThird = "third"

func TestNew(t *testing.T) {
	t.Parallel()

	m := New[string, int]()

	if m.Size() != 0 {
		t.Errorf("new map should have size 0, got %d", m.Size())
	}
}

func TestSetGet(t *testing.T) {
	t.Parallel()

	m := New[string, int]()

	m1 := m.Set("foo", 1)
	m2 := m1.Set("bar", 2)
	m3 := m2.Set("baz", 3)

	// Original unchanged
	if m.Size() != 0 {
		t.Errorf("original should be unchanged, got size %d", m.Size())
	}

	// Check values
	if val, ok := m3.Get("foo"); !ok || val != 1 {
		t.Errorf("expected foo=1, got %v, %v", val, ok)
	}

	if val, ok := m3.Get("bar"); !ok || val != 2 {
		t.Errorf("expected bar=2, got %v, %v", val, ok)
	}

	if val, ok := m3.Get("baz"); !ok || val != 3 {
		t.Errorf("expected baz=3, got %v, %v", val, ok)
	}

	if _, ok := m3.Get("missing"); ok {
		t.Error("expected missing key to return false")
	}

	// Check sizes
	if m1.Size() != 1 {
		t.Errorf("m1 size should be 1, got %d", m1.Size())
	}

	if m2.Size() != 2 {
		t.Errorf("m2 size should be 2, got %d", m2.Size())
	}

	if m3.Size() != 3 {
		t.Errorf("m3 size should be 3, got %d", m3.Size())
	}
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	m := New[string, int]()
	m1 := m.Set("key", 1)
	m2 := m1.Set("key", 2)

	// m1 should still have old value
	if val, _ := m1.Get("key"); val != 1 {
		t.Errorf("m1 should have key=1, got %d", val)
	}

	// m2 should have new value
	if val, _ := m2.Get("key"); val != 2 {
		t.Errorf("m2 should have key=2, got %d", val)
	}

	// Size should not change on update
	if m2.Size() != 1 {
		t.Errorf("size should be 1 after update, got %d", m2.Size())
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()

	m := New[string, int]()
	m1 := m.Set("a", 1).Set("b", 2).Set("c", 3)

	m2 := m1.Delete("b")

	// m1 unchanged
	if val, ok := m1.Get("b"); !ok || val != 2 {
		t.Errorf("m1 should still have b=2")
	}

	if m1.Size() != 3 {
		t.Errorf("m1 size should be 3, got %d", m1.Size())
	}

	// m2 has b removed
	if _, ok := m2.Get("b"); ok {
		t.Error("m2 should not have b")
	}

	if m2.Size() != 2 {
		t.Errorf("m2 size should be 2, got %d", m2.Size())
	}

	// Other keys intact
	if val, ok := m2.Get("a"); !ok || val != 1 {
		t.Errorf("m2 should have a=1")
	}

	if val, ok := m2.Get("c"); !ok || val != 3 {
		t.Errorf("m2 should have c=3")
	}
}

func TestDeleteMissing(t *testing.T) {
	t.Parallel()

	m := New[string, int]().Set("a", 1)
	m2 := m.Delete("missing")

	// Should return same map
	if m2 != m {
		t.Error("deleting missing key should return same map")
	}
}

func TestRange(t *testing.T) {
	t.Parallel()

	m := New[string, int]()
	m = m.Set("a", 1).Set("b", 2).Set("c", 3)

	found := make(map[string]int)

	m.Range(func(key string, val int) bool {
		found[key] = val

		return true
	})

	if len(found) != 3 {
		t.Errorf("expected 3 entries, got %d", len(found))
	}

	if found["a"] != 1 || found["b"] != 2 || found["c"] != 3 {
		t.Errorf("unexpected values: %v", found)
	}
}

func TestRangeEarlyStop(t *testing.T) {
	t.Parallel()

	const (
		iterationLimit = 100
		expectedCount  = 10
	)

	m := New[string, int]()

	for index := range iterationLimit {
		m = m.Set(fmt.Sprintf("key%d", index), index)
	}

	count := 0

	m.Range(func(_ string, _ int) bool {
		count++

		return count < expectedCount
	})

	if count != expectedCount {
		t.Errorf("expected %d iterations, got %d", expectedCount, count)
	}
}

func TestManyKeys(t *testing.T) {
	t.Parallel()

	m := New[string, int]()

	// Insert many keys
	const keyCount = 10000

	for index := range keyCount {
		m = m.Set(fmt.Sprintf("key%d", index), index)
	}

	if m.Size() != keyCount {
		t.Errorf("expected size %d, got %d", keyCount, m.Size())
	}

	// Verify all keys
	for index := range keyCount {
		key := fmt.Sprintf("key%d", index)

		if val, ok := m.Get(key); !ok || val != index {
			t.Errorf("expected %s=%d, got %v, %v", key, index, val, ok)
		}
	}
}

func TestStructuralSharing(t *testing.T) {
	t.Parallel()

	const baseKeyCount = 100

	m1 := New[string, int]()

	for index := range baseKeyCount {
		m1 = m1.Set(fmt.Sprintf("key%d", index), index)
	}

	const (
		updateKey   = 50
		updateValue = 999
	)

	// Create a new version with one change
	m2 := m1.Set(fmt.Sprintf("key%d", updateKey), updateValue)

	// Both should work independently
	if val, _ := m1.Get(fmt.Sprintf("key%d", updateKey)); val != updateKey {
		t.Errorf("m1 should have key50=50, got %d", val)
	}

	if val, _ := m2.Get(fmt.Sprintf("key%d", updateKey)); val != updateValue {
		t.Errorf("m2 should have key50=999, got %d", val)
	}

	// Other keys shared
	if val, _ := m2.Get("key0"); val != 0 {
		t.Errorf("m2 should have key0=0, got %d", val)
	}

	if val, _ := m2.Get("key99"); val != 99 {
		t.Errorf("m2 should have key99=99, got %d", val)
	}
}

func TestIntKeys(t *testing.T) {
	t.Parallel()

	m := New[int, string]()
	m = m.Set(1, "one").Set(2, "two").Set(3, "three")

	if val, ok := m.Get(2); !ok || val != "two" {
		t.Errorf("expected 2=two, got %v, %v", val, ok)
	}
}

func TestNilMap(t *testing.T) {
	t.Parallel()

	var m *HAMT[string, int]

	if m.Size() != 0 {
		t.Error("nil map size should be 0")
	}

	if _, ok := m.Get("key"); ok {
		t.Error("nil map get should return false")
	}

	// Set on nil should work
	m2 := m.Set("key", 1)

	if val, ok := m2.Get("key"); !ok || val != 1 {
		t.Errorf("expected key=1 after set on nil")
	}
}

func TestEmptyStringKey(t *testing.T) {
	t.Parallel()

	const expectedValue = 42

	m := New[string, int]()
	m = m.Set("", expectedValue)

	if val, ok := m.Get(""); !ok || val != expectedValue {
		t.Errorf("expected empty string key to work, got %v, %v", val, ok)
	}
}

// TestStructKeys verifies that non-primitive (struct) keys hash correctly
// and don't all collide to the same hash.
func TestStructKeys(t *testing.T) {
	t.Parallel()

	type Point struct {
		X, Y int
	}

	m := New[Point, string]()
	point1 := Point{1, 2}
	point2 := Point{3, 4}
	point3 := Point{1, 2} // Same as point1

	m = m.Set(point1, "first")
	m = m.Set(point2, "second")

	// point1 and point3 are equal, so setting point3 should update point1's value
	m = m.Set(point3, testValueThird)

	if m.Size() != 2 {
		t.Errorf("expected size 2 (point1==point3), got %d", m.Size())
	}

	if val, ok := m.Get(point1); !ok || val != testValueThird {
		t.Errorf("expected point1=third, got %v, %v", val, ok)
	}

	if val, ok := m.Get(point2); !ok || val != "second" {
		t.Errorf("expected point2=second, got %v, %v", val, ok)
	}

	if val, ok := m.Get(point3); !ok || val != testValueThird {
		t.Errorf("expected point3=third (same as point1), got %v, %v", val, ok)
	}
}

// TestPointerKeys verifies that pointer keys work correctly.
func TestPointerKeys(t *testing.T) {
	t.Parallel()

	type Data struct {
		Value int
	}

	m := New[*Data, string]()
	data1 := &Data{Value: 1}
	data2 := &Data{Value: 2}
	data3 := &Data{Value: 1} // Different pointer, same content

	m = m.Set(data1, "first")
	m = m.Set(data2, "second")
	m = m.Set(data3, testValueThird) // Different key from data1 (different pointer)

	if m.Size() != 3 {
		t.Errorf("expected size 3 (different pointers), got %d", m.Size())
	}

	if val, ok := m.Get(data1); !ok || val != "first" {
		t.Errorf("expected data1=first, got %v, %v", val, ok)
	}

	if val, ok := m.Get(data2); !ok || val != "second" {
		t.Errorf("expected data2=second, got %v, %v", val, ok)
	}

	if val, ok := m.Get(data3); !ok || val != testValueThird {
		t.Errorf("expected data3=third, got %v, %v", val, ok)
	}
}

// TestManyStructKeys ensures struct keys don't all collide.
func TestManyStructKeys(t *testing.T) {
	t.Parallel()

	type Coord struct {
		X, Y int
	}

	const (
		coordCount  = 1000
		coordModulo = 100
	)

	m := New[Coord, int]()

	for index := range coordCount {
		coord := Coord{X: index % coordModulo, Y: index / coordModulo}
		m = m.Set(coord, index)
	}

	if m.Size() != coordCount {
		t.Errorf("expected size %d, got %d (hash collisions?)", coordCount, m.Size())
	}

	// Verify all keys retrievable
	for index := range coordCount {
		coord := Coord{X: index % coordModulo, Y: index / coordModulo}

		if val, ok := m.Get(coord); !ok || val != index {
			t.Errorf("expected %v=%d, got %v, %v", coord, index, val, ok)
		}
	}
}

func TestFromMap(t *testing.T) {
	t.Parallel()

	original := map[string]int{"a": 1, "b": 2, "c": 3}
	m := FromMap(original)

	if m.Size() != 3 {
		t.Errorf("expected size 3, got %d", m.Size())
	}

	for key, val := range original {
		if got, ok := m.Get(key); !ok || got != val {
			t.Errorf("expected %s=%d, got %v, %v", key, val, got, ok)
		}
	}
}

func TestToMap(t *testing.T) {
	t.Parallel()

	const (
		valX = 10
		valY = 20
		valZ = 30
	)

	m := New[string, int]().Set("x", valX).Set("y", valY).Set("z", valZ)
	result := m.ToMap()

	if len(result) != 3 {
		t.Errorf("expected 3 entries, got %d", len(result))
	}

	if result["x"] != valX || result["y"] != valY || result["z"] != valZ {
		t.Errorf("unexpected values: %v", result)
	}
}

// Benchmarks

func BenchmarkSet(b *testing.B) {
	const keyModulo = 1000

	m := New[string, int]()

	b.ResetTimer()

	for index := range b.N {
		m = m.Set(fmt.Sprintf("key%d", index%keyModulo), index)
	}
}

func BenchmarkGet(b *testing.B) {
	const keyCount = 1000

	m := New[string, int]()

	for index := range keyCount {
		m = m.Set(fmt.Sprintf("key%d", index), index)
	}

	b.ResetTimer()

	for index := range b.N {
		m.Get(fmt.Sprintf("key%d", index%keyCount))
	}
}

func BenchmarkSetGet_Comparison(b *testing.B) {
	const baseKeyCount = 100

	b.Run("HAMT", func(b *testing.B) {
		m := New[string, int]()

		for index := range baseKeyCount {
			m = m.Set(fmt.Sprintf("key%d", index), index)
		}

		b.ResetTimer()

		for index := range b.N {
			m2 := m.Set("newkey", index)
			m2.Get("key50")
		}
	})

	b.Run("MapCopy", func(b *testing.B) {
		m := make(map[string]int)

		for index := range baseKeyCount {
			m[fmt.Sprintf("key%d", index)] = index
		}

		b.ResetTimer()

		for index := range b.N {
			// Simulate immutable update by copying
			m2 := make(map[string]int, len(m)+1)

			for key, val := range m {
				m2[key] = val
			}

			m2["newkey"] = index
			_ = m2["key50"]
		}
	})
}

func BenchmarkStructuralSharing(b *testing.B) {
	const (
		baseKeyCount = 1000
		verifyCount  = 10
		verifyKey    = 500
	)

	// Test that creating many versions doesn't blow up memory
	m := New[string, int]()

	for index := range baseKeyCount {
		m = m.Set(fmt.Sprintf("base%d", index), index)
	}

	b.ResetTimer()
	b.ReportAllocs()

	versions := make([]*HAMT[string, int], b.N)

	for index := range b.N {
		versions[index] = m.Set(fmt.Sprintf("version%d", index), index)
	}

	// Verify they're all valid
	for index := range min(verifyCount, b.N) {
		if val, ok := versions[index].Get(fmt.Sprintf("base%d", verifyKey)); !ok || val != verifyKey {
			b.Fatalf("version %d corrupted", index)
		}
	}
}
