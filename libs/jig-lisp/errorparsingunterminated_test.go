package lisp_test

import (
	"testing"

	"github.com/jig/lisp"
)

func TestBase(t *testing.T) {
	ast, err := lisp.READ(`"hello"`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ast.(string) != `hello` {
		t.Fatal(ast)
	}
}

func TestErrorParsingUnterminatedString(t *testing.T) {
	ast, err := lisp.READ(`"hello`, nil, nil)
	if err == nil {
		t.Fatalf("must throw error but returns %q", ast)
	}
	if err.Error() != `1: invalid token "hello` {
		t.Fatal(err)
	}
}

func TestErrorParsingUnterminatedHexa(t *testing.T) {
	ast, err := lisp.READ(`0xXYZ`, nil, nil)
	if err == nil {
		t.Fatalf("must throw error but returns %q", ast)
	}
	if err.Error() != `1: invalid token 0x` {
		t.Fatal(err)
	}
}
