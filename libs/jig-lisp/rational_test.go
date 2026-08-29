package lisp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/core/nscore"
	"github.com/jig/lisp/types"
)

// TestFractionLiteral verifies that a no-whitespace "3/4" reads as an exact
// rational while a bare "/" symbol and a spaced "3 /4" stay untouched.
func TestFractionLiteral(t *testing.T) {
	ast, err := lisp.READ("3/4", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := ast.(*types.Rat)
	if !ok {
		t.Fatalf("expected *types.Rat, got %T (%v)", ast, ast)
	}
	if got := r.String(); got != "3/4" {
		t.Fatalf("expected 3/4, got %s", got)
	}

	// Negative fraction written as -3/4 (sign folds into the numerator).
	ast, err = lisp.READ("-3/4", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, ok = ast.(*types.Rat)
	if !ok {
		t.Fatalf("expected *types.Rat, got %T (%v)", ast, ast)
	}
	if got := r.String(); got != "-3/4" {
		t.Fatalf("expected -3/4, got %s", got)
	}

	// A lone "/" symbol must not be merged.
	ast, err = lisp.READ("/", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ast.(types.Symbol); !ok {
		t.Fatalf("expected bare / symbol, got %T", ast)
	}

	// "3 /4" with whitespace is two separate forms, not a fraction literal:
	// the list still contains the symbols 3 and /4 (3 elements total).
	ast, err = lisp.READ("(list 3 /4)", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	lst, _ := types.GetSlice(ast)
	if len(lst) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(lst))
	}

	// Fraction literal with zero denominator is a reader error.
	if _, err := lisp.READ("1/0", nil, nil); err == nil {
		t.Fatal("expected error for 1/0 literal")
	}
}

// TestExactArithmetic verifies the rational foundation of the core arithmetic:
// division yields exact fractions, integral results come back as ints, and
// comparisons/equality work across int and Rat.
func TestExactArithmetic(t *testing.T) {
	e := env.NewEnv()
	if err := nscore.Load(e); err != nil {
		t.Fatal(err)
	}

	check := func(expr, want string) {
		t.Helper()
		got, err := lisp.REPL(context.Background(), e, expr, nil)
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		s, ok := got.(string)
		if !ok {
			t.Fatalf("%s: expected string result, got %T (%v)", expr, got, got)
		}
		if !strings.Contains(s, want) {
			t.Fatalf("%s = %s, want substring %q", expr, s, want)
		}
	}

	check("(/ 10 3)", "10/3")      // exact fraction, not integer division
	check("(+ 1/2 1/3)", "5/6")    // literal fractions combine exactly
	check("(/ 6 2)", "3")          // integral result normalizes back to int
	check("(+ 1 2 3 4)", "10")     // variadic +
	check("(- 5 2 1)", "2")        // variadic -
	check("(- 5)", "-5")           // unary negation
	check("(* 2 3 4)", "24")       // variadic *
	check("(= 1 1/1)", "true")     // int equals exact 1/1
	check("(= 1/2 1/2)", "true")   // same fraction
	check("(< 1/3 1/2)", "true")   // fraction ordering
	check("(>= 4/2 2)", "true")    // 4/2 == 2
	check("(number? 3/4)", "true") // number? accepts fractions
	check("(* 1/3 3)", "1")        // exact cancellation returns int
}
