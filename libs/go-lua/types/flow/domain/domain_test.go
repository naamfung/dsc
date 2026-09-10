package domain

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestClassifyAtom_HasType(t *testing.T) {
	atom := constraint.Atom{Kind: constraint.AtomKindHasType}
	if got := ClassifyAtom(atom); got != AtomClassType {
		t.Errorf("ClassifyAtom(HasType) = %v, want AtomClassType", got)
	}
}

func TestClassifyAtom_Truthy(t *testing.T) {
	atom := constraint.Atom{Kind: constraint.AtomKindTruthy}
	if got := ClassifyAtom(atom); got != AtomClassType {
		t.Errorf("ClassifyAtom(Truthy) = %v, want AtomClassType", got)
	}
}

func TestClassifyAtom_Lt(t *testing.T) {
	atom := constraint.Atom{Kind: constraint.AtomKindLt}
	if got := ClassifyAtom(atom); got != AtomClassNumeric {
		t.Errorf("ClassifyAtom(Lt) = %v, want AtomClassNumeric", got)
	}
}

func TestClassifyAtom_EqNil(t *testing.T) {
	atom := constraint.Atom{
		Kind:  constraint.AtomKindEq,
		Right: constraint.TermNil(),
	}
	if got := ClassifyAtom(atom); got != AtomClassType {
		t.Errorf("ClassifyAtom(Eq nil) = %v, want AtomClassType", got)
	}
}

func TestClassifyAtom_EqConst(t *testing.T) {
	atom := constraint.Atom{
		Kind:  constraint.AtomKindEq,
		Left:  constraint.TermConst(5),
		Right: constraint.TermConst(10),
	}
	if got := ClassifyAtom(atom); got != AtomClassNumeric {
		t.Errorf("ClassifyAtom(Eq const) = %v, want AtomClassNumeric", got)
	}
}
