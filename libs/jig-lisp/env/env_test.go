package env

import (
	"testing"

	"github.com/jig/lisp/types"
)

func TestEnv(t *testing.T) {
	year := types.Symbol{Val: "year"}
	ns := NewEnv()
	ns.Set(year, 1984)
	res, err := ns.Get(year)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int) != 1984 {
		t.Fatal()
	}
}

func TestSubordEnv(t *testing.T) {
	year := types.Symbol{Val: "year"}
	ns := NewEnv()

	s1env := NewSubordinateEnv(ns)
	s1env.Set(year, 1984)

	s2env := NewSubordinateEnv(ns)
	s2env.Set(year, 1985)

	res, err := s1env.Get(year)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int) != 1984 {
		t.Fatal()
	}

	res2, err := s2env.Get(year)
	if err != nil {
		t.Fatal(err)
	}
	if res2.(int) != 1985 {
		t.Fatal()
	}

	if _, err := ns.Get(year); err == nil {
		t.Fatal("should not find symbol")
	}
}
