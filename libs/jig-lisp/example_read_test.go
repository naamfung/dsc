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

func ExampleREAD() {
	ns := env.NewEnv()
	_ = nscore.Load(ns) // to load '+' function

	ast, err := lisp.READ(
		"(+ 1 1)",
		types.NewCursorFile("ExampleREAD"),
		ns,
	)
	if err != nil {
		log.Fatalf("read error: %s", err)
	}
	result, err := lisp.EVAL(
		context.Background(),
		ast,
		ns,
	)
	if err != nil {
		log.Fatalf("eval error: %s", err)
	}
	fmt.Printf("result: %d\n", result)

	// Output:
	// result: 2
}
