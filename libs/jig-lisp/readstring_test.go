package lisp

import (
	"testing"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/types"
)

func TestEscapeInStrings(t *testing.T) {
	ast, err := READ(`"\"\n\""`, nil, env.NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	if ast.(string) != "\"\n\"" {
		t.Fatal(err)
	}
}

func TestReadString(t *testing.T) {
	ast, err := READ(`(read-string "\"\n\"")`, nil, env.NewEnv())
	if err != nil {
		t.Fatal(err)
	}
	if ast.(types.List).Val[1].(string) != "\"\n\"" {
		t.Fatal(err)
	}
}
