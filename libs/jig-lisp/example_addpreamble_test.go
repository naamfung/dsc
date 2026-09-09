package lisp_test

import (
	"context"
	"fmt"
	"log"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/core/nscore"
	"github.com/jig/lisp/types"
)

func ExampleAddPreamble() {
	// incrementInLisp is sample function implemented in Lisp
	result, err := incrementInLisp(2)
	if err != nil {
		log.Fatalf("eval error: %s", err)
	}
	fmt.Printf("result: %d\n", result)

	// Output:
	// result: 3
}

const incrementInLispSourceCode = "(+ 1 $ARG)"

func incrementInLisp(arg int) (int, error) {
	ns := env.NewEnv()
	_ = nscore.Load(ns) // to load '+' function

	preamble := map[string]types.MalType{
		"$ARG": arg,
	}
	sourceCode, err := lisp.AddPreamble(
		incrementInLispSourceCode,
		preamble,
	)
	if err != nil {
		return 0, err
	}
	ast, err := lisp.READWithPreamble(
		sourceCode,
		types.NewCursorFile("ExampleREAD"),
		ns,
	)
	if err != nil {
		return 0, err
	}
	result, err := lisp.EVAL(
		context.Background(),
		ast,
		ns,
	)
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}
