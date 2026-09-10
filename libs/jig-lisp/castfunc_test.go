package lisp

import (
	"context"
	_ "embed"
	"strings"
	"testing"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/core"
	"github.com/jig/lisp/types"
)

//go:embed castfunc_test.lisp
var castfunc_test string

func TestCastFunc(t *testing.T) {
	repl_env := env.NewEnv()
	core.Load(repl_env)
	ctx := context.Background()
	ast, err := READ(castfunc_test, types.NewCursorFile(castfunc_test), repl_env)
	if err != nil {
		t.Fatal(err)
	}
	_, err = EVAL(ctx, ast, repl_env)
	if err == nil {
		t.Fatal(err)
	}
	if !strings.Contains(err.Error(), "attempt to call non-function (was of type int)") {
		t.Fatal("test failed")
	}
}
