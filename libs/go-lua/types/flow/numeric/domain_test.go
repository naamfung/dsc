package numeric

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestNewDomain(t *testing.T) {
	d := NewDomain(constraint.Env{})
	if d == nil {
		t.Fatal("NewDomain returned nil")
	}
	if d.state == nil {
		t.Error("NewDomain should initialize state")
	}
	if d.theory == nil {
		t.Error("NewDomain should initialize theory")
	}
}

func TestDomain_IsUnsat_Empty(t *testing.T) {
	d := NewDomain(constraint.Env{})
	if d.IsUnsat() {
		t.Error("fresh domain should not be unsat")
	}
}

func TestDomain_Clone(t *testing.T) {
	d := NewDomain(constraint.Env{})
	clone := d.Clone()
	if clone == nil {
		t.Fatal("Clone returned nil")
	}
	if clone == d {
		t.Error("Clone should return a different instance")
	}
}

func TestDomain_State(t *testing.T) {
	d := NewDomain(constraint.Env{})
	if d.State() != d.state {
		t.Error("State() should return internal state")
	}
}

func TestDomain_Theory(t *testing.T) {
	d := NewDomain(constraint.Env{})
	if d.Theory() != d.theory {
		t.Error("Theory() should return internal theory")
	}
}

func TestDomain_ApplyAtom_Nil(t *testing.T) {
	d := &Domain{state: nil}
	result := d.ApplyAtom(constraint.Atom{})
	if result {
		t.Error("ApplyAtom with nil state should return false")
	}
}

func TestDomain_Join(t *testing.T) {
	d1 := NewDomain(constraint.Env{})
	d2 := NewDomain(constraint.Env{})
	joined := d1.Join(d2)
	if joined == nil {
		t.Fatal("Join returned nil")
	}
}

func TestDomain_TightenWithTheory(t *testing.T) {
	d := NewDomain(constraint.Env{})
	tightened := d.TightenWithTheory()
	if tightened == nil {
		t.Fatal("TightenWithTheory returned nil")
	}
}
