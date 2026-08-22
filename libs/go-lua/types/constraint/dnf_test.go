package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/narrow"
)

// conditionContains checks if any disjunct contains the given constraint.
func conditionContains(c Condition, ct Constraint) bool {
	for _, d := range c.Disjuncts {
		if ConjunctionContains(d, ct) {
			return true
		}
	}
	return false
}

func TestDNF_NestedAndOr(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}
	pathZ := Path{Root: "z", Symbol: 3}

	// (A AND B) OR C
	a := FromConstraints(Truthy{Path: pathX})
	b := FromConstraints(Truthy{Path: pathY})
	c := FromConstraints(Truthy{Path: pathZ})

	ab := And(a, b)
	result := Or(ab, c)

	if len(result.Disjuncts) != 2 {
		t.Errorf("(A AND B) OR C should have 2 disjuncts, got %d", len(result.Disjuncts))
	}
}

func TestDNF_DistributiveProperty(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}
	pathZ := Path{Root: "z", Symbol: 3}

	// A AND (B OR C) = (A AND B) OR (A AND C)
	a := FromConstraints(Truthy{Path: pathX})
	b := FromConstraints(Truthy{Path: pathY})
	c := FromConstraints(Truthy{Path: pathZ})

	bc := Or(b, c)
	result := And(a, bc)

	if len(result.Disjuncts) != 2 {
		t.Errorf("A AND (B OR C) should have 2 disjuncts, got %d", len(result.Disjuncts))
	}

	for _, d := range result.Disjuncts {
		if !ConjunctionContains(d, Truthy{Path: pathX}) {
			t.Error("each disjunct should contain Truthy{x}")
		}
	}
}

func TestDNF_DeMorgan_NotAnd(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}

	// NOT(A AND B) = NOT(A) OR NOT(B)
	c := FromConstraints(Truthy{Path: pathX}, Truthy{Path: pathY})
	neg := Not(c)

	if len(neg.Disjuncts) != 2 {
		t.Errorf("NOT(A AND B) should have 2 disjuncts, got %d", len(neg.Disjuncts))
	}

	var foundFalsyX, foundFalsyY bool
	for _, d := range neg.Disjuncts {
		if ConjunctionContains(d, Falsy{Path: pathX}) {
			foundFalsyX = true
		}
		if ConjunctionContains(d, Falsy{Path: pathY}) {
			foundFalsyY = true
		}
	}
	if !foundFalsyX || !foundFalsyY {
		t.Errorf("NOT(A AND B) should produce Falsy{x} OR Falsy{y}, x=%v y=%v", foundFalsyX, foundFalsyY)
	}
}

func TestDNF_DeMorgan_NotOr(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}

	// NOT(A OR B) = NOT(A) AND NOT(B)
	a := FromConstraints(Truthy{Path: pathX})
	b := FromConstraints(Truthy{Path: pathY})
	ab := Or(a, b)
	neg := Not(ab)

	if len(neg.Disjuncts) != 1 {
		t.Errorf("NOT(A OR B) should have 1 disjunct, got %d", len(neg.Disjuncts))
	}

	if len(neg.Disjuncts) > 0 {
		d := neg.Disjuncts[0]
		if !ConjunctionContains(d, Falsy{Path: pathX}) || !ConjunctionContains(d, Falsy{Path: pathY}) {
			t.Error("NOT(A OR B) should contain both Falsy{x} AND Falsy{y}")
		}
	}
}

func TestDNF_DeMorgan_DoubleNegation(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}

	c := FromConstraints(Truthy{Path: pathX})
	negNeg := Not(Not(c))

	if !c.Equals(negNeg) {
		t.Errorf("NOT(NOT(A)) should equal A")
	}
}

func TestDNF_DeMorgan_ComplexExpression(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}
	pathZ := Path{Root: "z", Symbol: 3}

	// NOT((A AND B) OR C) = NOT(A AND B) AND NOT(C) = (NOT(A) OR NOT(B)) AND NOT(C)
	a := FromConstraints(Truthy{Path: pathX})
	b := FromConstraints(Truthy{Path: pathY})
	c := FromConstraints(Truthy{Path: pathZ})

	ab := And(a, b)
	expr := Or(ab, c)
	neg := Not(expr)

	// Result should have 2 disjuncts: (Falsy{x} AND Falsy{z}) OR (Falsy{y} AND Falsy{z})
	if len(neg.Disjuncts) != 2 {
		t.Errorf("NOT((A AND B) OR C) should have 2 disjuncts, got %d", len(neg.Disjuncts))
	}

	for _, d := range neg.Disjuncts {
		if !ConjunctionContains(d, Falsy{Path: pathZ}) {
			t.Error("each disjunct should contain Falsy{z}")
		}
	}
}

func TestDNF_MultiDisjunct_MustConstraints(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	// (A AND B) OR (A AND C) -> must constraint is A
	a := FromConstraints(NotNil{Path: path})
	b := FromConstraints(HasType{Path: path, Type: narrow.BuiltinTypeKey("string")})
	c := FromConstraints(HasType{Path: path, Type: narrow.BuiltinTypeKey("number")})

	ab := And(a, b)
	ac := And(a, c)
	result := Or(ab, ac)

	must := result.MustConstraints()
	if len(must) != 1 {
		t.Errorf("expected 1 must constraint, got %d", len(must))
	}
	if !ConjunctionContains(must, NotNil{Path: path}) {
		t.Error("must constraint should be NotNil{x}")
	}
}

func TestDNF_ThreeWayOr(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}

	a := FromConstraints(HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")})
	b := FromConstraints(HasType{Path: pathX, Type: narrow.BuiltinTypeKey("number")})
	c := FromConstraints(HasType{Path: pathX, Type: narrow.BuiltinTypeKey("boolean")})

	ab := Or(a, b)
	result := Or(ab, c)

	if len(result.Disjuncts) != 3 {
		t.Errorf("A OR B OR C should have 3 disjuncts, got %d", len(result.Disjuncts))
	}
}

func TestDNF_ThreeWayAnd(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}
	pathZ := Path{Root: "z", Symbol: 3}

	a := FromConstraints(Truthy{Path: pathX})
	b := FromConstraints(Truthy{Path: pathY})
	c := FromConstraints(Truthy{Path: pathZ})

	ab := And(a, b)
	result := And(ab, c)

	if len(result.Disjuncts) != 1 {
		t.Errorf("A AND B AND C should have 1 disjunct, got %d", len(result.Disjuncts))
	}
	if len(result.Disjuncts[0]) != 3 {
		t.Errorf("conjunction should have 3 constraints, got %d", len(result.Disjuncts[0]))
	}
}

func TestDNF_MultiDisjunct_ContainsConstraint(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	strType := HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}
	numType := HasType{Path: path, Type: narrow.BuiltinTypeKey("number")}
	boolType := HasType{Path: path, Type: narrow.BuiltinTypeKey("boolean")}

	a := FromConstraints(strType)
	b := FromConstraints(numType)
	result := Or(a, b)

	if !conditionContains(result, strType) {
		t.Error("condition should contain string type constraint")
	}
	if !conditionContains(result, numType) {
		t.Error("condition should contain number type constraint")
	}
	if conditionContains(result, boolType) {
		t.Error("condition should not contain boolean type constraint")
	}
}

func TestDNF_ComplexNested(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}
	pathZ := Path{Root: "z", Symbol: 3}
	pathW := Path{Root: "w", Symbol: 4}

	// ((A AND B) OR C) AND D
	a := FromConstraints(Truthy{Path: pathX})
	b := FromConstraints(Truthy{Path: pathY})
	c := FromConstraints(Truthy{Path: pathZ})
	d := FromConstraints(Truthy{Path: pathW})

	ab := And(a, b)
	abOrC := Or(ab, c)
	result := And(abOrC, d)

	// Expected: (A AND B AND D) OR (C AND D)
	if len(result.Disjuncts) != 2 {
		t.Errorf("((A AND B) OR C) AND D should have 2 disjuncts, got %d", len(result.Disjuncts))
	}

	for _, disj := range result.Disjuncts {
		if !ConjunctionContains(disj, Truthy{Path: pathW}) {
			t.Error("each disjunct should contain Truthy{w}")
		}
	}
}

func TestDNF_SubsumptionElimination(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	// (A) OR (A AND B) should simplify to (A) due to subsumption
	a := FromConstraints(Truthy{Path: path})
	b := FromConstraints(Truthy{Path: path}, NotNil{Path: path})
	result := Or(a, b)

	// The more specific disjunct (A AND B) is subsumed by A
	if len(result.Disjuncts) != 1 {
		t.Errorf("subsumption should reduce to 1 disjunct, got %d", len(result.Disjuncts))
	}
}

func TestDNF_DuplicateElimination(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	a := FromConstraints(Truthy{Path: path})
	result := Or(a, a)

	if len(result.Disjuncts) != 1 {
		t.Errorf("duplicate disjuncts should be eliminated, got %d", len(result.Disjuncts))
	}
}

func TestDNF_Idempotent(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	a := FromConstraints(Truthy{Path: path})

	// A AND A = A
	andResult := And(a, a)
	if !andResult.Equals(a) {
		t.Error("A AND A should equal A")
	}

	// A OR A = A
	orResult := Or(a, a)
	if !orResult.Equals(a) {
		t.Error("A OR A should equal A")
	}
}

func TestDNF_Absorption(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}

	// A OR (A AND B) = A (absorption law)
	a := FromConstraints(Truthy{Path: pathX})
	b := FromConstraints(Truthy{Path: pathY})
	ab := And(a, b)
	result := Or(a, ab)

	// The more specific (A AND B) is subsumed by A
	if len(result.Disjuncts) != 1 {
		t.Errorf("absorption should reduce to 1 disjunct, got %d", len(result.Disjuncts))
	}
}

func TestDNF_ConstraintDiversity(t *testing.T) {
	pathX := Path{Root: "x", Symbol: 1}
	pathY := Path{Root: "y", Symbol: 2}

	// Mix different constraint types in DNF
	strType := HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}
	notNil := NotNil{Path: pathY}
	truthy := Truthy{Path: pathX}

	a := FromConstraints(strType, notNil)
	b := FromConstraints(truthy)
	result := Or(a, b)

	if len(result.Disjuncts) != 2 {
		t.Errorf("expected 2 disjuncts, got %d", len(result.Disjuncts))
	}

	all := result.AllConstraints()
	if len(all) != 3 {
		t.Errorf("expected 3 total constraints, got %d", len(all))
	}
}

func TestDNF_EmptyConjunctionInteraction(t *testing.T) {
	path := Path{Root: "x", Symbol: 1}

	a := FromConstraints(Truthy{Path: path})

	// True AND A = A
	result := And(TrueCondition(), a)
	if !result.Equals(a) {
		t.Error("True AND A should equal A")
	}

	// True OR A = True
	result = Or(TrueCondition(), a)
	if !result.IsTrue() {
		t.Error("True OR A should be True")
	}

	// False AND A = False
	result = And(FalseCondition(), a)
	if !result.IsFalse() {
		t.Error("False AND A should be False")
	}

	// False OR A = A
	result = Or(FalseCondition(), a)
	if !result.Equals(a) {
		t.Error("False OR A should equal A")
	}
}

func TestDNF_FourWayExpression(t *testing.T) {
	pathA := Path{Root: "a", Symbol: 1}
	pathB := Path{Root: "b", Symbol: 2}
	pathC := Path{Root: "c", Symbol: 3}
	pathD := Path{Root: "d", Symbol: 4}

	// (A OR B) AND (C OR D) = (A AND C) OR (A AND D) OR (B AND C) OR (B AND D)
	a := FromConstraints(Truthy{Path: pathA})
	b := FromConstraints(Truthy{Path: pathB})
	c := FromConstraints(Truthy{Path: pathC})
	d := FromConstraints(Truthy{Path: pathD})

	ab := Or(a, b)
	cd := Or(c, d)
	result := And(ab, cd)

	if len(result.Disjuncts) != 4 {
		t.Errorf("(A OR B) AND (C OR D) should have 4 disjuncts, got %d", len(result.Disjuncts))
	}

	for _, disj := range result.Disjuncts {
		if len(disj) != 2 {
			t.Errorf("each disjunct should have 2 constraints, got %d", len(disj))
		}
	}
}
