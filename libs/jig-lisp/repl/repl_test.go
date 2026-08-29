package repl

import (
	"context"
	"testing"

	"github.com/jig/lisp"
	"github.com/jig/lisp/env"
	"github.com/jig/lisp/types"
)

func TestMultiline(t *testing.T) {
	for _, partialLine := range []string{"(", "{", "[", "#{", "«"} {
		ns := env.NewEnv()
		_, err := lisp.REPL(context.Background(), ns, partialLine, types.NewCursorFile("REPL TEST"))
		if err == nil {
			t.Fatal("test failed")
		}
		if multiLine(err) {
			continue
		}
		t.Fatalf("test failed for %s", partialLine)
	}
}

func TestMultilineFail(t *testing.T) {
	for _, partialLine := range []string{`"Hello"`, ":a", "1", "-1"} {
		ns := env.NewEnv()
		_, err := lisp.REPL(context.Background(), ns, partialLine, types.NewCursorFile("REPL TEST"))
		if err != nil {
			t.Fatal("test failed")
		}
	}
}
