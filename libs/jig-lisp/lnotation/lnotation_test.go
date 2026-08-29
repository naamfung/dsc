package lnotation

import (
	"context"
	"log"
	"testing"

	"github.com/jig/lisp"
	. "github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/assert/nsassert"
	"github.com/jig/lisp/lib/concurrent/nsconcurrent"
	"github.com/jig/lisp/lib/core/nscore"
	"github.com/jig/lisp/lib/coreextented/nscoreextended"
	. "github.com/jig/lisp/types"
)

// (range 0 4)

func TestLNotationMinimalExample(t *testing.T) {
	sampleCode := LS("range", 0, 4)
	lr, err := lisp.EVAL(context.TODO(), sampleCode, NewTestEnv())
	if err != nil {
		t.Fatal(err)
	}
	vec, ok := lr.(Vector)
	if !ok {
		t.Fatal("not a vector")
	}
	for i := 0; i < 4; i++ {
		if vec.Val[i] != i {
			t.Fatalf("not %d", i)
		}
	}
}

// (reduce + 0 (range 0 10000))

func TestLNotation(t *testing.T) {
	sampleCode := LS("reduce", S("+"), 0, LS("range", 0, 10000))
	lr, err := lisp.EVAL(context.TODO(), sampleCode, NewTestEnv())
	if err != nil {
		t.Fatal(err)
	}
	switch lr := lr.(type) {
	case int:
		if lr != 49995000 {
			t.Fatal("incorrect result")
		}
	default:
		t.Fatal("incorrect type")
	}
}

// (do
//		(def fib (fn [n]
//			(if (= n 0)
//			1
//			(if (= n 1)
//				1
//				(+ (fib (- n 1))
//				(fib (- n 2)))))))
//		(fib 50))

func TestLNotationFibonacci(t *testing.T) {
	// use of a mix of L() and LS()
	do := S("do")
	def := S("def")
	fib := S("fib")
	fn := S("fn")
	n := S("n")
	iF := S("if")

	env := NewTestEnv()
	lr, err := lisp.EVAL(context.TODO(),
		L(do,
			L(def, fib, L(fn, V([]Symbol{n}),
				L(iF, LS("=", n, 0),
					1,
					L(iF, LS("=", n, 1),
						1,
						LS("+", L(fib, LS("-", n, 1)),
							L(fib, LS("-", n, 2))))))),
			L(fib, 15)),
		env,
	)
	if err != nil {
		t.Fatal(err)
	}
	if lr.(int) != 987 {
		t.Fatal("wrong result for fibonacci")
	}
}

func NewTestEnv() EnvType {
	repl_env := NewEnv()

	for _, library := range []struct {
		name string
		load func(repl_env EnvType) error
	}{
		{"core mal", nscore.Load},
		{"core mal with input", nscore.LoadInput},
		// {"command line args", nscore.LoadCmdLineArgs(command.PreParseArgs(os.Args))} // if needed
		{"concurrent", nsconcurrent.Load},
		{"core mal extended", nscoreextended.Load},
		{"assert", nsassert.Load},
	} {
		if err := library.load(repl_env); err != nil {
			log.Fatalf("Library Load Error: %v\n", err)
		}
	}
	return repl_env
}
