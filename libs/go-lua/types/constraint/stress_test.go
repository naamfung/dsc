package constraint

import (
	"math/rand"
	"testing"

	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDNFCap_ExceedsLimit_NoPanic(t *testing.T) {
	// Create 100 disjuncts (exceeds DefaultMaxDisjuncts = 32)
	var disjuncts [][]Constraint
	for i := 0; i < 100; i++ {
		disjuncts = append(disjuncts, []Constraint{
			FieldEquals{
				Target: Path{Root: "x", Symbol: 1},
				Field:  "id",
				Value:  typ.LiteralInt(int64(i)),
			},
		})
	}

	// Should not panic
	cond := FromDisjuncts(disjuncts)

	if cond.IsFalse() {
		t.Error("condition should not be false")
	}

	if cond.NumDisjuncts() > DefaultMaxDisjuncts {
		t.Errorf("expected at most %d disjuncts after cap, got %d", DefaultMaxDisjuncts, cond.NumDisjuncts())
	}

	t.Logf("100 input disjuncts -> %d after normalization", cond.NumDisjuncts())
}

func TestDNFCap_ANDExplosion_Capped(t *testing.T) {
	// Create two conditions with many disjuncts each
	// AND would normally create 10 * 10 = 100 disjuncts
	var disjuncts1, disjuncts2 [][]Constraint
	for i := 0; i < 10; i++ {
		disjuncts1 = append(disjuncts1, []Constraint{
			FieldEquals{Target: Path{Root: "x", Symbol: 1}, Field: "a", Value: typ.LiteralInt(int64(i))},
		})
		disjuncts2 = append(disjuncts2, []Constraint{
			FieldEquals{Target: Path{Root: "x", Symbol: 1}, Field: "b", Value: typ.LiteralInt(int64(i))},
		})
	}

	cond1 := FromDisjuncts(disjuncts1)
	cond2 := FromDisjuncts(disjuncts2)

	result := And(cond1, cond2)

	if result.IsFalse() {
		t.Error("AND result should not be false")
	}

	if result.NumDisjuncts() > DefaultMaxDisjuncts {
		t.Errorf("AND explosion not capped: got %d disjuncts, max is %d", result.NumDisjuncts(), DefaultMaxDisjuncts)
	}

	t.Logf("10x10 AND -> %d disjuncts", result.NumDisjuncts())
}

func TestDNFCap_Stable(t *testing.T) {
	var disjuncts [][]Constraint
	for i := 0; i < 50; i++ {
		disjuncts = append(disjuncts, []Constraint{
			HasType{Path: Path{Root: "x", Symbol: 1}, Type: narrow.BuiltinTypeKey("string")},
		})
	}

	// Run multiple times
	var baseline int
	for i := 0; i < 10; i++ {
		cond := FromDisjuncts(disjuncts)
		if i == 0 {
			baseline = cond.NumDisjuncts()
		} else if cond.NumDisjuncts() != baseline {
			t.Errorf("run %d: got %d disjuncts, expected %d", i, cond.NumDisjuncts(), baseline)
		}
	}
}

func TestDNFCap_MustConstraintsPreserved(t *testing.T) {
	// Create disjuncts that all share a common constraint
	path := Path{Root: "x", Symbol: 1}
	var disjuncts [][]Constraint
	for i := 0; i < 50; i++ {
		disjuncts = append(disjuncts, []Constraint{
			NotNil{Path: path}, // Common to all
			FieldEquals{Target: path, Field: "id", Value: typ.LiteralInt(int64(i))},
		})
	}

	cond := FromDisjuncts(disjuncts)

	// MustConstraints should include the common NotNil
	must := cond.MustConstraints()
	hasNotNil := false
	for _, c := range must {
		if _, ok := c.(NotNil); ok {
			hasNotNil = true
			break
		}
	}

	if !hasNotNil {
		t.Error("MustConstraints should preserve common NotNil constraint")
	}
}

func TestRandomized_ConstraintOrdering(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	path := Path{Root: "x", Symbol: 1}
	constraints := []Constraint{
		NotNil{Path: path},
		HasType{Path: path, Type: narrow.BuiltinTypeKey("string")},
		FieldEquals{Target: path, Field: "tag", Value: typ.LiteralString("a")},
	}

	// Get baseline
	baseline := FromConstraints(constraints...)
	baselineHash := baseline.Hash()

	// Shuffle and verify same hash
	for i := 0; i < 20; i++ {
		shuffled := make([]Constraint, len(constraints))
		copy(shuffled, constraints)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		result := FromConstraints(shuffled...)
		if result.Hash() != baselineHash {
			t.Errorf("iteration %d: hash mismatch after shuffle", i)
		}
	}
}

func TestRandomized_DisjunctOrdering(t *testing.T) {
	rng := rand.New(rand.NewSource(123))

	path := Path{Root: "x", Symbol: 1}
	var disjuncts [][]Constraint
	for i := 0; i < 10; i++ {
		disjuncts = append(disjuncts, []Constraint{
			FieldEquals{Target: path, Field: "id", Value: typ.LiteralInt(int64(i))},
		})
	}

	baseline := FromDisjuncts(disjuncts)
	baselineDisjuncts := baseline.NumDisjuncts()

	// Shuffle and verify same disjunct count
	for i := 0; i < 20; i++ {
		shuffled := make([][]Constraint, len(disjuncts))
		copy(shuffled, disjuncts)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		result := FromDisjuncts(shuffled)
		if result.NumDisjuncts() != baselineDisjuncts {
			t.Errorf("iteration %d: disjunct count mismatch: got %d, want %d", i, result.NumDisjuncts(), baselineDisjuncts)
		}
	}
}

func TestDeepPath_Creation(t *testing.T) {
	// Create path with 20 segments
	path := Path{Root: "x", Symbol: 1}
	for i := 0; i < 20; i++ {
		path = path.Append(Segment{Kind: SegmentField, Name: "level"})
	}

	if len(path.Segments) != 20 {
		t.Errorf("expected 20 segments, got %d", len(path.Segments))
	}

	// Key should be stable
	key1 := path.Key()
	key2 := path.Key()
	if key1 != key2 {
		t.Error("key should be stable across calls")
	}
}

func TestDeepPath_ConstraintApplication(t *testing.T) {
	// Create deep path
	path := Path{Root: "x", Symbol: 1}
	for i := 0; i < 10; i++ {
		path = path.Append(Segment{Kind: SegmentField, Name: "level"})
	}

	// Create constraint on deep path
	c := FieldEquals{Target: path, Field: "tag", Value: typ.LiteralString("test")}

	// Should not panic
	cond := FromConstraints(c)

	if cond.IsFalse() {
		t.Error("condition should not be false")
	}
	if !cond.HasConstraints() {
		t.Error("condition should have constraints")
	}
}

func TestLargeUnion_TypeKeyResolution(t *testing.T) {
	// Create 100 type keys
	keys := make([]narrow.TypeKey, 100)
	for i := 0; i < 100; i++ {
		keys[i] = narrow.BuiltinTypeKey("type" + string(rune('A'+i%26)))
	}

	path := Path{Root: "x", Symbol: 1}

	// Create HasType constraints for each
	var constraints []Constraint
	for _, key := range keys {
		constraints = append(constraints, HasType{Path: path, Type: key})
	}

	// Create OR of all
	var disjuncts [][]Constraint
	for _, c := range constraints {
		disjuncts = append(disjuncts, []Constraint{c})
	}

	cond := FromDisjuncts(disjuncts)

	// Should be capped
	if cond.NumDisjuncts() > DefaultMaxDisjuncts {
		t.Errorf("expected capped disjuncts, got %d", cond.NumDisjuncts())
	}
}

func TestAlgebra_AndAssociative(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	a := FromConstraints(NotNil{Path: path})
	b := FromConstraints(HasType{Path: path, Type: narrow.BuiltinTypeKey("string")})
	c := FromConstraints(Truthy{Path: path})

	ab := And(a, b)
	abThenC := And(ab, c)

	bc := And(b, c)
	aThenBC := And(a, bc)

	if abThenC.NumDisjuncts() != aThenBC.NumDisjuncts() {
		t.Errorf("AND not associative: (%d disjuncts) vs (%d disjuncts)", abThenC.NumDisjuncts(), aThenBC.NumDisjuncts())
	}
}

func TestAlgebra_OrCommutative(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	a := FromConstraints(FieldEquals{Target: path, Field: "tag", Value: typ.LiteralString("a")})
	b := FromConstraints(FieldEquals{Target: path, Field: "tag", Value: typ.LiteralString("b")})

	ab := Or(a, b)
	ba := Or(b, a)

	if ab.NumDisjuncts() != ba.NumDisjuncts() {
		t.Errorf("OR not commutative: %d vs %d disjuncts", ab.NumDisjuncts(), ba.NumDisjuncts())
	}
}

func TestAlgebra_NotNot(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	original := FromConstraints(NotNil{Path: path})

	notOnce := Not(original)
	notTwice := Not(notOnce)

	// NOT(NOT(x)) should have same constraint as x
	// Though representation may differ
	if notTwice.IsFalse() && !original.IsFalse() {
		t.Error("NOT(NOT(x)) should not be false when x is not false")
	}
}

func TestEdgeCase_EmptyDisjuncts(t *testing.T) {
	cond := FromDisjuncts(nil)
	if !cond.IsTrue() {
		t.Error("empty disjuncts should be true")
	}
}

func TestEdgeCase_EmptyConjunction(t *testing.T) {
	cond := FromConstraints()
	if !cond.IsTrue() {
		t.Error("empty conjunction should be true")
	}
}

func TestEdgeCase_SingleDisjunct(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}
	disjuncts := [][]Constraint{
		{NotNil{Path: path}},
	}

	cond := FromDisjuncts(disjuncts)

	if cond.NumDisjuncts() != 1 {
		t.Errorf("expected 1 disjunct, got %d", cond.NumDisjuncts())
	}
}

func TestEdgeCase_DuplicateConstraints(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	// Same constraint repeated
	constraints := []Constraint{
		NotNil{Path: path},
		NotNil{Path: path},
		NotNil{Path: path},
	}

	cond := FromConstraints(constraints...)

	// Should deduplicate
	if cond.NumDisjuncts() != 1 {
		t.Errorf("expected 1 disjunct, got %d", cond.NumDisjuncts())
	}

	disjunct := cond.DisjunctConstraints(0)
	if len(disjunct) > 1 {
		t.Errorf("expected deduplication, got %d constraints", len(disjunct))
	}
}
