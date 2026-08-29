package core

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	. "github.com/jig/lisp/types"
)

// Exact rational arithmetic for the core + - * / functions.
//
// Every operand is an exact number (int or Rat); floats are rejected so exact
// results are never silently polluted by approximations. Each operator is
// variadic with Clojure semantics: (+) -> 0, (*) -> 1, (/) and (-) require at
// least one argument, and a single argument is returned unchanged. Results are
// normalized back to int whenever they are integral and fit a platform int
// (via NormRat), preserving existing integer behavior and indexes.

func coreAdd(_ context.Context, a ...MalType) (MalType, error) {
	acc := new(big.Rat)
	for _, arg := range a {
		r, ok := ToBigRat(arg)
		if !ok {
			return nil, typeErr("+", arg)
		}
		acc.Add(acc, r)
	}
	return NormRat(acc), nil
}

func coreSub(_ context.Context, a ...MalType) (MalType, error) {
	if len(a) == 0 {
		return nil, errors.New("- expects at least 1 argument")
	}
	acc, ok := ToBigRat(a[0])
	if !ok {
		return nil, typeErr("-", a[0])
	}
	if len(a) == 1 {
		return NormRat(acc.Neg(acc)), nil
	}
	for _, arg := range a[1:] {
		r, ok := ToBigRat(arg)
		if !ok {
			return nil, typeErr("-", arg)
		}
		acc.Sub(acc, r)
	}
	return NormRat(acc), nil
}

func coreMul(_ context.Context, a ...MalType) (MalType, error) {
	acc := new(big.Rat).SetInt64(1)
	for _, arg := range a {
		r, ok := ToBigRat(arg)
		if !ok {
			return nil, typeErr("*", arg)
		}
		acc.Mul(acc, r)
	}
	return NormRat(acc), nil
}

func coreDiv(_ context.Context, a ...MalType) (MalType, error) {
	if len(a) == 0 {
		return nil, errors.New("/ expects at least 1 argument")
	}
	acc, ok := ToBigRat(a[0])
	if !ok {
		return nil, typeErr("/", a[0])
	}
	for _, arg := range a[1:] {
		r, ok := ToBigRat(arg)
		if !ok {
			return nil, typeErr("/", arg)
		}
		if r.Sign() == 0 {
			return nil, errors.New("division by zero")
		}
		acc.Quo(acc, r)
	}
	return NormRat(acc), nil
}

func typeErr(op string, v MalType) error {
	return fmt.Errorf("arithmetic operation %s on non-number (%T)", op, v)
}

// Ordered comparison helpers. Unlike arithmetic, comparisons accept floats too
// (converted to their exact rational value) so "<" keeps working across exact
// numbers as well as float escape-hatch results.

func cmpVals(a, b MalType) (int, error) {
	ra, ok := ToBigRatAny(a)
	if !ok {
		return 0, fmt.Errorf("comparison on non-number (%T)", a)
	}
	rb, ok := ToBigRatAny(b)
	if !ok {
		return 0, fmt.Errorf("comparison on non-number (%T)", b)
	}
	return ra.Cmp(rb), nil
}

func numLt(_ context.Context, a, b MalType) (bool, error) {
	c, err := cmpVals(a, b)
	return c < 0, err
}

func numLe(_ context.Context, a, b MalType) (bool, error) {
	c, err := cmpVals(a, b)
	return c <= 0, err
}

func numGt(_ context.Context, a, b MalType) (bool, error) {
	c, err := cmpVals(a, b)
	return c > 0, err
}

func numGe(_ context.Context, a, b MalType) (bool, error) {
	c, err := cmpVals(a, b)
	return c >= 0, err
}
