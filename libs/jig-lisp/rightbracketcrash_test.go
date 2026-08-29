package lisp

import (
	_ "embed"
	"testing"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lisperror"
)

func TestRightBracketCrash(t *testing.T) {
	_, err := READ(")", nil, env.NewEnv())
	if err == nil {
		t.Fatal("error must not be nill")
	}
	if errT, ok := err.(lisperror.LispError); ok && errT.ErrorValue().(error).Error() != `unexpected ')'` {
		t.Fatalf("%q != %q", `unexpected ')'`, errT.ErrorValue())
	}
}
