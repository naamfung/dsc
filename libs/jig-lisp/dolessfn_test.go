package lisp_test

import (
	"context"
	_ "embed"
	"log"
	"testing"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/core/nscore"
	"github.com/jig/lisp/types"
)

//go:embed dolessfn_test.lisp
var dolessfn_test string

func TestDoLessFunction(t *testing.T) {
	ns := env.NewEnv()
	for _, library := range []struct {
		name string
		load func(ns types.EnvType) error
	}{
		{"core mal", nscore.Load},
		// {"core mal with input", nscore.LoadInput},
		// {"command line args", nscore.LoadCmdLineArgs},
		// {"concurrent", nsconcurrent.Load},
		// {"core mal extended", nscoreextended.Load},
		// {"assert", nsassert.Load},
	} {
		if err := library.load(ns); err != nil {
			log.Fatalf("Library Load Error: %v\n", err)
		}
	}

	ast, err := lisp.READ(dolessfn_test, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := lisp.EVAL(context.Background(), ast, ns)
	if err != nil {
		t.Fatal(err)
	}
	if !res.(bool) {
		t.Fatalf(`res.(int) != 4 (res.(int) == %d)`, res)
	}
}
