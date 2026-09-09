package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

func TestIntersectionEmpty(t *testing.T) {
	i := NewIntersection()
	if i != Any {
		t.Error("empty intersection should be Any")
	}
}

func TestIntersectionSingle(t *testing.T) {
	i := NewIntersection(Number)
	if i != Number {
		t.Error("single-member intersection should unwrap to member")
	}
}

func TestIntersectionBasic(t *testing.T) {
	i := NewIntersection(Number, String)

	if i.Kind() != kind.Intersection {
		t.Errorf("Kind: got %v, want Intersection", i.Kind())
	}

	inter := i.(*Intersection)
	if len(inter.Members) != 2 {
		t.Errorf("Members: got %d, want 2", len(inter.Members))
	}
}

func TestIntersectionWithNever(t *testing.T) {
	i := NewIntersection(Number, Never)
	if i != Never {
		t.Error("intersection containing Never should collapse to Never")
	}
}

func TestIntersectionAnyIdentity(t *testing.T) {
	i := NewIntersection(Number, Any, String)

	inter := i.(*Intersection)
	for _, m := range inter.Members {
		if m.Kind() == kind.Any {
			t.Error("Any should be filtered from intersection")
		}
	}
}

func TestIntersectionFlattening(t *testing.T) {
	inner := NewIntersection(Number, String)
	outer := NewIntersection(inner, Boolean)

	inter := outer.(*Intersection)
	if len(inter.Members) != 3 {
		t.Errorf("nested intersection should flatten, got %d members", len(inter.Members))
	}
}

func TestIntersectionDeduplication(t *testing.T) {
	i := NewIntersection(Number, String, Number)

	inter := i.(*Intersection)
	if len(inter.Members) != 2 {
		t.Errorf("duplicate should be removed, got %d members", len(inter.Members))
	}
}

func TestIntersectionDedupHashCollision(t *testing.T) {
	a := &fakeType{id: "a", hash: 101}
	b := &fakeType{id: "b", hash: 101}

	i := NewIntersection(a, b).(*Intersection)
	if len(i.Members) != 2 {
		t.Errorf("hash collision should keep both members, got %d", len(i.Members))
	}
}

func TestIntersectionEquality(t *testing.T) {
	i1 := NewIntersection(Number, String)
	i2 := NewIntersection(Number, String)
	i3 := NewIntersection(Number, Boolean)

	if !i1.Equals(i2) {
		t.Error("number & string should equal number & string")
	}

	if i1.Equals(i3) {
		t.Error("number & string should not equal number & boolean")
	}

	if i1.Hash() != i2.Hash() {
		t.Error("equal intersections should have same hash")
	}
}

func TestIntersectionOrderIndependence(t *testing.T) {
	i1 := NewIntersection(Number, String)
	i2 := NewIntersection(String, Number)

	if !i1.Equals(i2) {
		t.Error("intersection order should not affect equality")
	}

	if i1.Hash() != i2.Hash() {
		t.Error("intersection order should not affect hash")
	}
}

func TestIntersectionNotEqualToPrimitive(t *testing.T) {
	i := NewIntersection(Number, String)
	if i.Equals(Number) {
		t.Error("intersection should not equal primitive")
	}
}

func TestIntersectionString(t *testing.T) {
	i := NewIntersection(Number, String).(*Intersection)

	s := i.String()
	if s == "" {
		t.Error("intersection String() should not be empty")
	}
}
