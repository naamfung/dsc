package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/narrow"
)

func TestCondition_TrueFalse(t *testing.T) {
	trueCond := TrueCondition()
	if !trueCond.IsTrue() {
		t.Error("TrueCondition should be true")
	}
	if trueCond.IsFalse() {
		t.Error("TrueCondition should not be false")
	}
	if trueCond.HasConstraints() {
		t.Error("TrueCondition should have no constraints")
	}
	if len(trueCond.MustConstraints()) != 0 {
		t.Error("TrueCondition should have empty must constraints")
	}

	falseCond := FalseCondition()
	if !falseCond.IsFalse() {
		t.Error("FalseCondition should be false")
	}
	if falseCond.IsTrue() {
		t.Error("FalseCondition should not be true")
	}
	if falseCond.HasConstraints() {
		t.Error("FalseCondition should have no constraints")
	}
	if len(falseCond.MustConstraints()) != 0 {
		t.Error("FalseCondition should have empty must constraints")
	}
}

func TestCondition_FromConstraints(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	c := FromConstraints()
	if !c.IsTrue() {
		t.Error("FromConstraints() with no items should be true")
	}

	c = FromConstraints(Truthy{Path: path})
	if len(c.Disjuncts) != 1 {
		t.Errorf("single constraint should produce 1 disjunct, got %d", len(c.Disjuncts))
	}
	if len(c.MustConstraints()) != 1 {
		t.Errorf("single constraint should produce 1 must constraint, got %d", len(c.MustConstraints()))
	}

	c = FromConstraints(Truthy{Path: path}, NotNil{Path: path})
	if len(c.Disjuncts) != 1 {
		t.Errorf("multiple constraints should produce 1 disjunct, got %d", len(c.Disjuncts))
	}
	if len(c.MustConstraints()) != 2 {
		t.Errorf("expected 2 must constraints, got %d", len(c.MustConstraints()))
	}
}

func TestCondition_AndOr(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}

	a := FromConstraints(HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")})
	b := FromConstraints(NotNil{Path: pathY})

	// True AND A = A
	if got := And(TrueCondition(), a); !got.Equals(a) {
		t.Error("True AND A should equal A")
	}
	// False AND A = False
	if got := And(FalseCondition(), a); !got.IsFalse() {
		t.Error("False AND A should be false")
	}

	// A AND B
	ab := And(a, b)
	if len(ab.Disjuncts) != 1 {
		t.Errorf("A AND B should produce 1 disjunct, got %d", len(ab.Disjuncts))
	}
	if len(ab.MustConstraints()) != 2 {
		t.Errorf("A AND B should have 2 must constraints, got %d", len(ab.MustConstraints()))
	}

	// A OR B
	or := Or(a, b)
	if len(or.Disjuncts) != 2 {
		t.Errorf("A OR B should produce 2 disjuncts, got %d", len(or.Disjuncts))
	}
	if len(or.MustConstraints()) != 0 {
		t.Errorf("A OR B should have no must constraints, got %d", len(or.MustConstraints()))
	}
}

func TestCondition_MustConstraintsCommon(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	setA := NewConjunction(HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}, NotNil{Path: path})
	setB := NewConjunction(HasType{Path: path, Type: narrow.BuiltinTypeKey("number")}, NotNil{Path: path})

	a := FromConjunction(setA)
	b := FromConjunction(setB)
	or := Or(a, b)

	must := or.MustConstraints()
	if len(must) != 1 {
		t.Errorf("expected 1 common constraint, got %d", len(must))
	}
	if !ConjunctionContains(must, NotNil{Path: path}) {
		t.Error("must constraints should contain NotNil")
	}
}

func TestCondition_Not(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}

	c := FromConstraints(Truthy{Path: pathX}, Truthy{Path: pathY})
	neg := Not(c)
	if len(neg.Disjuncts) != 2 {
		t.Errorf("NOT of 2-conjunction should produce 2 disjuncts, got %d", len(neg.Disjuncts))
	}
	if len(neg.MustConstraints()) != 0 {
		t.Errorf("NOT should have no must constraints, got %d", len(neg.MustConstraints()))
	}
}

func TestCondition_Substitute_EmptyArgs(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}
	cond := FromConstraints(NotNil{Path: path})

	subst := cond.Substitute(nil)
	if !subst.Equals(cond) {
		t.Errorf("Substitute(nil) should preserve non-placeholder constraints: got %v", subst)
	}
}

func TestCondition_Substitute_DropUnboundPlaceholder(t *testing.T) {
	cond := FromConstraints(NotNil{Path: Path{Root: "$0"}})

	subst := cond.Substitute(nil)
	if !subst.IsTrue() {
		t.Errorf("Substitute(nil) on placeholder-only constraints should yield true, got %v", subst)
	}
}

func TestCondition_SizeCap(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}
	var disjuncts [][]Constraint
	for i := 0; i < DefaultMaxDisjuncts+5; i++ {
		disjuncts = append(disjuncts, NewConjunction(HasType{
			Path: path,
			Type: narrow.BuiltinTypeKey("type" + string(rune('a'+i%26))),
		}))
	}

	a := normalizeCondition(Condition{Disjuncts: disjuncts[:DefaultMaxDisjuncts/2]})
	b := normalizeCondition(Condition{Disjuncts: disjuncts[DefaultMaxDisjuncts/2:]})
	result := Or(a, b)
	if len(result.Disjuncts) > DefaultMaxDisjuncts {
		t.Errorf("OR exceeding cap should not exceed %d disjuncts, got %d", DefaultMaxDisjuncts, len(result.Disjuncts))
	}
}

func TestNegateConstraint(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	tests := []struct {
		input    Constraint
		expected Constraint
	}{
		{Truthy{Path: path}, Falsy{Path: path}},
		{Falsy{Path: path}, Truthy{Path: path}},
		{IsNil{Path: path}, NotNil{Path: path}},
		{NotNil{Path: path}, IsNil{Path: path}},
		{HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}, NotHasType{Path: path, Type: narrow.BuiltinTypeKey("string")}},
		{NotHasType{Path: path, Type: narrow.BuiltinTypeKey("string")}, HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}},
	}

	for _, tt := range tests {
		result, ok := NegateConstraint(tt.input)
		if !ok || result == nil {
			t.Errorf("NegateConstraint(%T) returned nil", tt.input)
			continue
		}
		if !result.Equals(tt.expected) {
			t.Errorf("NegateConstraint(%T) = %T, want %T", tt.input, result, tt.expected)
		}
	}
}

func TestCondition_Equals(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}
	set := NewConjunction(Truthy{Path: path})

	a := FromConjunction(set)
	b := FromConjunction(set)
	if !a.Equals(b) {
		t.Error("same conditions should be equal")
	}

	c := FromConjunction(NewConjunction(Falsy{Path: path}))
	if a.Equals(c) {
		t.Error("different conditions should not be equal")
	}
}

func TestKeyOf_HashAndEquals(t *testing.T) {
	table := Path{Root: "t", Symbol: 1}
	key := Path{Root: "k", Symbol: 2}
	table2 := Path{Root: "t2", Symbol: 3}

	ko1 := KeyOf{Table: table, Key: key}
	ko2 := KeyOf{Table: table, Key: key}
	ko3 := KeyOf{Table: table2, Key: key}

	if ko1.Kind() != KindKeyOf {
		t.Error("KeyOf should have KindKeyOf")
	}

	if ko1.Hash() != ko2.Hash() {
		t.Error("identical KeyOf constraints should have same hash")
	}

	if !ko1.Equals(ko2) {
		t.Error("identical KeyOf constraints should be equal")
	}

	if ko1.Equals(ko3) {
		t.Error("different KeyOf constraints should not be equal")
	}

	paths := ko1.Paths()
	if len(paths) != 2 {
		t.Errorf("KeyOf should return 2 paths, got %d", len(paths))
	}
}

func TestKeyOf_Substitute(t *testing.T) {
	placeholder0 := Path{Root: "$0"}
	placeholder1 := Path{Root: "$1"}
	actual0 := Path{Root: "table", Symbol: 1}
	actual1 := Path{Root: "key", Symbol: 2}

	ko := KeyOf{Table: placeholder0, Key: placeholder1}
	subst, ok := ko.Substitute([]Path{actual0, actual1})
	if !ok {
		t.Fatal("Substitute should succeed")
	}

	koSubst, ok := subst.(KeyOf)
	if !ok {
		t.Fatal("Substitute should return KeyOf")
	}

	if !koSubst.Table.Equal(actual0) {
		t.Error("Table path should be substituted")
	}
	if !koSubst.Key.Equal(actual1) {
		t.Error("Key path should be substituted")
	}
}

func TestKeyOf_InCondition(t *testing.T) {
	table := Path{Root: "t", Symbol: 1}
	key := Path{Root: "k", Symbol: 2}

	ko := KeyOf{Table: table, Key: key}
	cond := FromConstraints(ko)

	if len(cond.Disjuncts) != 1 {
		t.Errorf("expected 1 disjunct, got %d", len(cond.Disjuncts))
	}
	if len(cond.MustConstraints()) != 1 {
		t.Errorf("expected 1 must constraint, got %d", len(cond.MustConstraints()))
	}

	// Test HasKeyOfConstraint
	if !HasKeyOfConstraint(cond, table, key, nil) {
		t.Error("HasKeyOfConstraint should find KeyOf constraint")
	}

	otherKey := Path{Root: "other", Symbol: 3}
	if HasKeyOfConstraint(cond, table, otherKey, nil) {
		t.Error("HasKeyOfConstraint should not find KeyOf constraint with different key")
	}
}

func TestKeyOf_DNF_MustBeInAllDisjuncts(t *testing.T) {
	table := Path{Root: "t", Symbol: 1}
	key := Path{Root: "k", Symbol: 2}
	other := Path{Root: "x", Symbol: 3}

	ko := KeyOf{Table: table, Key: key}

	// (KeyOf AND A) - single disjunct, KeyOf is guaranteed
	condA := FromConstraints(ko, NotNil{Path: other})
	if !HasKeyOfConstraint(condA, table, key, nil) {
		t.Error("KeyOf should be found in single disjunct")
	}

	// (KeyOf AND A) OR (KeyOf AND B) - KeyOf in all disjuncts, guaranteed
	condB := FromConstraints(ko, Truthy{Path: other})
	condBoth := Or(condA, condB)
	if !HasKeyOfConstraint(condBoth, table, key, nil) {
		t.Error("KeyOf should be found when present in all disjuncts")
	}

	// (KeyOf AND A) OR (B) - KeyOf NOT in all disjuncts, NOT guaranteed
	condNoKeyOf := FromConstraints(NotNil{Path: other})
	condPartial := Or(condA, condNoKeyOf)
	if HasKeyOfConstraint(condPartial, table, key, nil) {
		t.Error("KeyOf should NOT be found when missing from some disjuncts")
	}
}

func TestKeyOf_EmptyAndFalseConditions(t *testing.T) {
	table := Path{Root: "t", Symbol: 1}
	key := Path{Root: "k", Symbol: 2}

	// TrueCondition - no constraints
	if HasKeyOfConstraint(TrueCondition(), table, key, nil) {
		t.Error("TrueCondition should not have KeyOf")
	}

	// FalseCondition - unsatisfiable
	if HasKeyOfConstraint(FalseCondition(), table, key, nil) {
		t.Error("FalseCondition should not have KeyOf")
	}
}

func TestKeyOf_WithPathResolver(t *testing.T) {
	table1 := Path{Root: "t", Symbol: 1}
	table2 := Path{Root: "t", Symbol: 1}
	key1 := Path{Root: "k", Symbol: 2}
	key2 := Path{Root: "k", Symbol: 2}

	ko := KeyOf{Table: table1, Key: key1}
	cond := FromConstraints(ko)

	// Resolver that maps paths to canonical keys
	resolver := func(p Path) PathKey {
		return p.Key()
	}

	// Same paths should match with resolver
	if !HasKeyOfConstraint(cond, table2, key2, resolver) {
		t.Error("Should find KeyOf with resolver matching same paths")
	}

	// Different symbol should not match
	differentTable := Path{Root: "t", Symbol: 99}
	if HasKeyOfConstraint(cond, differentTable, key1, resolver) {
		t.Error("Should not find KeyOf with different symbol")
	}
}

func TestKeyOf_String(t *testing.T) {
	table := Path{Root: "t", Symbol: 1}
	key := Path{Root: "k", Symbol: 2}

	ko := KeyOf{Table: table, Key: key}
	s := ko.String()
	if s == "" {
		t.Error("KeyOf.String() should not be empty")
	}
}

func TestKeyOf_AndOperation(t *testing.T) {
	table := Path{Root: "t", Symbol: 1}
	key := Path{Root: "k", Symbol: 2}
	other := Path{Root: "x", Symbol: 3}

	koCond := FromConstraints(KeyOf{Table: table, Key: key})
	otherCond := FromConstraints(NotNil{Path: other})

	// KeyOf AND NotNil should preserve KeyOf
	andCond := And(koCond, otherCond)
	if !HasKeyOfConstraint(andCond, table, key, nil) {
		t.Error("AND should preserve KeyOf constraint")
	}
}

func TestKeyOf_DifferentKey(t *testing.T) {
	table := Path{Root: "t", Symbol: 1}
	key1 := Path{Root: "k1", Symbol: 2}
	key2 := Path{Root: "k2", Symbol: 3}

	ko1 := KeyOf{Table: table, Key: key1}
	ko2 := KeyOf{Table: table, Key: key2}

	if ko1.Hash() == ko2.Hash() {
		t.Error("KeyOf with different keys should have different hashes")
	}

	if ko1.Equals(ko2) {
		t.Error("KeyOf with different keys should not be equal")
	}
}

func TestKeyOf_Negation(t *testing.T) {
	table := Path{Root: "t", Symbol: 1}
	key := Path{Root: "k", Symbol: 2}

	ko := KeyOf{Table: table, Key: key}
	neg, ok := NegateConstraint(ko)
	if ok {
		t.Errorf("KeyOf should not be negatable, but got %v", neg)
	}
	if neg != nil {
		t.Error("Negation of KeyOf should return nil")
	}
}

func TestKeyOf_NotWithOtherConstraint(t *testing.T) {
	table := Path{Root: "t", Symbol: 1}
	key := Path{Root: "k", Symbol: 2}

	ko := KeyOf{Table: table, Key: key}
	cond := FromConstraints(ko)

	// Negating condition with KeyOf
	notCond := Not(cond)

	// NOT(KeyOf) should become TrueCondition since KeyOf can't be negated
	if !notCond.IsTrue() {
		t.Error("NOT(KeyOf) should be TrueCondition since KeyOf cannot be negated")
	}
}

func TestKeyOf_Substitute_PartialPlaceholder(t *testing.T) {
	// Only table is placeholder
	placeholder := Path{Root: "$0"}
	actualKey := Path{Root: "key", Symbol: 2}
	actualTable := Path{Root: "table", Symbol: 1}

	ko := KeyOf{Table: placeholder, Key: actualKey}
	subst, ok := ko.Substitute([]Path{actualTable})
	if !ok {
		t.Fatal("Substitute should succeed with partial placeholder")
	}

	koSubst, ok := subst.(KeyOf)
	if !ok {
		t.Fatal("Substitute should return KeyOf")
	}

	if !koSubst.Table.Equal(actualTable) {
		t.Error("Table path should be substituted")
	}
	if !koSubst.Key.Equal(actualKey) {
		t.Error("Key path should remain unchanged")
	}
}

func TestKeyOf_Substitute_OutOfBounds(t *testing.T) {
	placeholder0 := Path{Root: "$0"}
	placeholder5 := Path{Root: "$5"} // Out of bounds

	ko := KeyOf{Table: placeholder0, Key: placeholder5}
	_, ok := ko.Substitute([]Path{{Root: "t", Symbol: 1}}) // Only 1 arg
	if ok {
		t.Error("Substitute should fail when placeholder is out of bounds")
	}
}

func TestKeyOf_SubstituteConjunction_PartialSubstitution(t *testing.T) {
	placeholder0 := Path{Root: "$0"}
	placeholder1 := Path{Root: "$1"}
	actual0 := Path{Root: "table", Symbol: 1}

	ko := KeyOf{Table: placeholder0, Key: placeholder1}
	conj := NewConjunction(ko)

	// Partial substitution: only $0 is provided
	result := SubstituteConjunction(conj, []Path{actual0})

	// Should preserve constraint with unresolved placeholder
	if len(result) != 1 {
		t.Fatalf("Expected 1 constraint after partial substitution, got %d", len(result))
	}

	koResult, ok := result[0].(KeyOf)
	if !ok {
		t.Fatalf("Expected KeyOf constraint, got %T", result[0])
	}

	// Table should be substituted
	if !koResult.Table.Equal(actual0) {
		t.Errorf("Table should be substituted to actual0, got %s", koResult.Table.String())
	}

	// Key should still be placeholder (preserved)
	if koResult.Key.Root != "$1" {
		t.Errorf("Key should remain as placeholder $1, got %s", koResult.Key.String())
	}
}

func TestKeyOf_SubstituteConjunction_BothUnresolved(t *testing.T) {
	placeholder0 := Path{Root: "$0"}
	placeholder1 := Path{Root: "$1"}

	ko := KeyOf{Table: placeholder0, Key: placeholder1}
	conj := NewConjunction(ko)

	// No args provided - both placeholders unresolved
	result := SubstituteConjunction(conj, nil)

	// Constraint should be dropped
	if len(result) != 0 {
		t.Errorf("Expected constraint to be dropped when both placeholders unresolved, got %d", len(result))
	}
}

func TestCondition_Subsumes(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}

	truthy := FromConstraints(Truthy{Path: pathX})
	notNil := FromConstraints(NotNil{Path: pathX})
	both := FromConstraints(Truthy{Path: pathX}, NotNil{Path: pathX})
	differentPath := FromConstraints(Truthy{Path: pathY})

	tests := []struct {
		name     string
		a        Condition
		b        Condition
		expected bool
	}{
		{"true subsumes anything", TrueCondition(), truthy, true},
		{"true subsumes true", TrueCondition(), TrueCondition(), true},
		{"true subsumes false", TrueCondition(), FalseCondition(), true},
		{"false only subsumes false", FalseCondition(), FalseCondition(), true},
		{"false does not subsume true", FalseCondition(), TrueCondition(), false},
		{"false does not subsume constraint", FalseCondition(), truthy, false},
		{"constraint does not subsume true", truthy, TrueCondition(), false},
		{"single subsumes more constrained", truthy, both, true},
		{"more constrained does not subsume single", both, truthy, false},
		{"same constraint subsumes itself", truthy, truthy, true},
		{"different constraints don't subsume", truthy, notNil, false},
		{"different paths don't subsume", truthy, differentPath, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Subsumes(tc.b); got != tc.expected {
				t.Errorf("Subsumes() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestCondition_Subsumes_Disjunctions(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}

	a := FromConstraints(Truthy{Path: pathX})
	b := FromConstraints(NotNil{Path: pathY})

	or := Or(a, b)

	if !a.Subsumes(a) {
		t.Error("a should subsume itself")
	}
	if !or.Subsumes(a) {
		t.Error("(a OR b) should subsume a")
	}
	if !or.Subsumes(b) {
		t.Error("(a OR b) should subsume b")
	}
	if a.Subsumes(or) {
		t.Error("a should not subsume (a OR b)")
	}
}

func TestCondition_NegateConstraint(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	tests := []struct {
		name       string
		constraint Constraint
		wantKind   Kind
	}{
		{"Truthy negates to Falsy", Truthy{Path: path}, KindFalsy},
		{"Falsy negates to Truthy", Falsy{Path: path}, KindTruthy},
		{"IsNil negates to NotNil", IsNil{Path: path}, KindNotNil},
		{"NotNil negates to IsNil", NotNil{Path: path}, KindIsNil},
		{"HasType negates to NotHasType", HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}, KindNotHasType},
		{"NotHasType negates to HasType", NotHasType{Path: path, Type: narrow.BuiltinTypeKey("string")}, KindHasType},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			negated, ok := NegateConstraint(tc.constraint)
			if !ok {
				t.Fatal("NegateConstraint returned false")
			}
			if negated.Kind() != tc.wantKind {
				t.Errorf("Kind = %v, want %v", negated.Kind(), tc.wantKind)
			}
		})
	}
}

func TestCondition_NegateConstraint_EqPath(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}

	eq := NewEqPath(pathX, pathY)
	negated, ok := NegateConstraint(eq)
	if !ok {
		t.Fatal("NegateConstraint returned false")
	}
	if negated.Kind() != KindNotEqPath {
		t.Errorf("Kind = %v, want KindNotEqPath", negated.Kind())
	}

	neq := NewNotEqPath(pathX, pathY)
	negated, ok = NegateConstraint(neq)
	if !ok {
		t.Fatal("NegateConstraint returned false")
	}
	if negated.Kind() != KindEqPath {
		t.Errorf("Kind = %v, want KindEqPath", negated.Kind())
	}
}

func TestCondition_NegateConstraint_Unsupported(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	unsupported := []Constraint{
		HasField{Path: path, Field: "f"},
		KeyOf{Table: path, Key: path},
	}

	for _, c := range unsupported {
		_, ok := NegateConstraint(c)
		if ok {
			t.Errorf("NegateConstraint(%T) should return false", c)
		}
	}
}

func TestCondition_NegateConstraint_FieldIndex(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	tests := []struct {
		name       string
		constraint Constraint
		wantKind   Kind
	}{
		{"FieldEquals negates to FieldNotEquals", FieldEquals{Target: path, Field: "f"}, KindFieldNotEquals},
		{"FieldNotEquals negates to FieldEquals", FieldNotEquals{Target: path, Field: "f"}, KindFieldEquals},
		{"IndexEquals negates to IndexNotEquals", IndexEquals{Target: path}, KindIndexNotEquals},
		{"IndexNotEquals negates to IndexEquals", IndexNotEquals{Target: path}, KindIndexEquals},
		{"FieldEqualsPath negates to FieldNotEqualsPath", FieldEqualsPath{Target: path, Field: "f", Value: path}, KindFieldNotEqualsPath},
		{"FieldNotEqualsPath negates to FieldEqualsPath", FieldNotEqualsPath{Target: path, Field: "f", Value: path}, KindFieldEqualsPath},
		{"IndexEqualsPath negates to IndexNotEqualsPath", IndexEqualsPath{Target: path, Value: path}, KindIndexNotEqualsPath},
		{"IndexNotEqualsPath negates to IndexEqualsPath", IndexNotEqualsPath{Target: path, Value: path}, KindIndexEqualsPath},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			negated, ok := NegateConstraint(tc.constraint)
			if !ok {
				t.Fatal("NegateConstraint returned false")
			}
			if negated.Kind() != tc.wantKind {
				t.Errorf("Kind = %v, want %v", negated.Kind(), tc.wantKind)
			}
		})
	}
}

func TestConjunctionContains(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}
	truthy := Truthy{Path: path}
	notNil := NotNil{Path: path}
	falsy := Falsy{Path: path}

	conj := []Constraint{truthy, notNil}

	if !ConjunctionContains(conj, truthy) {
		t.Error("conjunction should contain truthy")
	}
	if !ConjunctionContains(conj, notNil) {
		t.Error("conjunction should contain notNil")
	}
	if ConjunctionContains(conj, falsy) {
		t.Error("conjunction should not contain falsy")
	}
	if ConjunctionContains(nil, truthy) {
		t.Error("nil conjunction should not contain anything")
	}
	if ConjunctionContains([]Constraint{}, truthy) {
		t.Error("empty conjunction should not contain anything")
	}
}

func TestCondition_DisjunctConstraints(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}
	truthy := Truthy{Path: path}
	notNil := NotNil{Path: path}

	c := FromConstraints(truthy, notNil)

	dc := c.DisjunctConstraints(0)
	if len(dc) != 2 {
		t.Errorf("DisjunctConstraints(0) = %d constraints, want 2", len(dc))
	}

	dc = c.DisjunctConstraints(1)
	if dc != nil {
		t.Error("DisjunctConstraints(1) should return nil for out of bounds")
	}
}

func TestCondition_NumDisjuncts(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}
	a := FromConstraints(Truthy{Path: path})
	b := FromConstraints(NotNil{Path: path})

	if a.NumDisjuncts() != 1 {
		t.Errorf("NumDisjuncts = %d, want 1", a.NumDisjuncts())
	}

	or := Or(a, b)
	if or.NumDisjuncts() != 2 {
		t.Errorf("NumDisjuncts = %d, want 2", or.NumDisjuncts())
	}

	if TrueCondition().NumDisjuncts() != 1 {
		t.Errorf("TrueCondition should have 1 disjunct (empty conjunction), got %d", TrueCondition().NumDisjuncts())
	}

	if FalseCondition().NumDisjuncts() != 0 {
		t.Error("FalseCondition should have 0 disjuncts")
	}
}
