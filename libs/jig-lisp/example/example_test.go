package example

import (
	"context"
	"fmt"
	"log"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/concurrent/nsconcurrent"
	"github.com/jig/lisp/lib/core/nscore"
	"github.com/jig/lisp/lib/coreextented/nscoreextended"
	"github.com/jig/lisp/lib/system/nssystem"
	"github.com/jig/lisp/types"
)

func ExampleEVAL() {
	newEnv := env.NewEnv()

	// Load required lisp libraries
	for _, library := range []struct {
		name string
		load func(newEnv types.EnvType) error
	}{
		{"core mal", nscore.Load},
		{"core mal with input", nscore.LoadInput},
		// {"command line args", nscore.LoadCmdLineArgs(command.PreParseArgs(os.Args))} // if needed
		{"concurrent", nsconcurrent.Load},
		{"core mal extended", nscoreextended.Load},
		{"system", nssystem.Load},
	} {
		if err := library.load(newEnv); err != nil {
			log.Fatalf("Library Load Error %q: %v", library.name, err)
		}
	}

	// parse (READ) lisp code
	ast, err := lisp.READ(`(+ 2 2)`, nil, newEnv)
	if err != nil {
		log.Fatalf("READ error: %v", err)
	}

	// eval AST
	result, err := lisp.EVAL(context.TODO(), ast, newEnv)
	if err != nil {
		log.Fatalf("EVAL error: %v", err)
	}

	// use result
	if result.(int) != 4 {
		log.Fatalf("Result check error: %v", err)
	}

	// optionally print resulting AST
	fmt.Println(lisp.PRINT(result))
	// Output: 4
}
