package lisp_test

import (
	"fmt"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/core/nscore"
	"github.com/jig/lisp/types"
)

func Example_configInLisp() {
	ns := env.NewEnv()
	_ = nscore.Load(ns)
	config, _ := lisp.READ(
		`{
			:sessions 10
		}`,
		types.NewCursorFile("ExampleFunctionInLisp"),
		ns,
	)

	fmt.Println("sessions:", config.(types.HashMap).Val[types.NewKeyword("sessions")])

	// Output:
	// sessions: 10
}
