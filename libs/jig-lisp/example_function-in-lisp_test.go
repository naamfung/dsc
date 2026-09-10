package lisp_test

import (
	"context"
	"fmt"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/core/nscore"
	"github.com/jig/lisp/lnotation"
	"github.com/jig/lisp/types"
)

func Example_functionInLisp() {
	ns := env.NewEnv()
	_ = nscore.Load(ns)
	ast, _ := lisp.READ(
		`(fn [a] (* 10 a))`,
		types.NewCursorFile("ExampleFunctionInLisp"),
		ns,
	)

	res, _ := lisp.EVAL(context.Background(), ast, ns)
	functionInLisp := func(a int) (int, error) {
		res, err := lisp.EVAL(context.Background(), lnotation.L(res, a), ns)
		if err != nil {
			return 0, err
		}
		return res.(int), nil
	}

	result, _ := functionInLisp(3)
	fmt.Println("result:", result)

	// Output:
	// result: 30
}
