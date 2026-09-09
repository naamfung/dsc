package lisp

import (
	"context"
	"testing"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/core"
	"github.com/jig/lisp/lisperror"
	"github.com/jig/lisp/types"
)

func TestDefnMacroDontPanic(t *testing.T) {
	ns := env.NewEnv()
	ctx := context.Background()
	_, err := REPL(ctx, ns,
		// correct definition: "(defmacro defn (fn [name params body] `(def ~name (fn ~params ~body))))",
		// definition below (from Clojure) is not supported and must fail but... not panic as it does on v0.2.9
		"(defmacro defn [name params body] `(def ~name (fn ~params ~body)))",
		types.NewCursorFile(t.Name()),
	)
	var errMsg string
	if lispErr, ok := err.(lisperror.LispError); ok {
		errMsg = lispErr.ErrorValue().(error).Error()
	} else {
		errMsg = err.Error()
	}

	if errMsg != "symbol 'name' not found" {
		t.Fatalf("Expected error 'symbol 'name' not found', got: %s", errMsg)
	}
}

func TestDefnMacroQuasiquoteUnquoteSpliceUnquoteDontPanic(t *testing.T) {
	ns := env.NewEnv()
	ctx := context.Background()
	_, err := REPL(ctx, ns,
		// alternative correct definition: "(defmacro defn (fn [name params & body] (quasiquote (def (unquote name) (fn (unquote params) (splice-unquote body))))))",
		// definition below (from Clojure) is not supported and must fail but... not panic as it does on v0.2.9
		"(defmacro defn [name params & body] (quasiquote (def (unquote name) (fn (unquote params) (splice-unquote body)))))",
		types.NewCursorFile(t.Name()),
	)

	var errMsg string
	if lispErr, ok := err.(lisperror.LispError); ok {
		errMsg = lispErr.ErrorValue().(error).Error()
	} else {
		errMsg = err.Error()
	}

	if errMsg != "symbol 'name' not found" {
		t.Fatalf("Expected error 'symbol 'name' not found', got: %s", errMsg)
	}
}

// let's check here the intended use for that macro defn
func TestDefnMacro(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns) // need cons

	ctx := context.Background()
	result, err := REPL(ctx, ns,
		"(do"+
			"(defmacro defn (fn [name params body] `(def ~name (fn ~params ~body))))\n"+
			"(defn sum [a b] (+ a b))\n"+
			"(sum 1 2))",
		types.NewCursorFile(t.Name()),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.(string) != "3" {
		t.Fatal("test failed")
	}
}

func TestDefnMacro2(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns) // need cons

	ctx := context.Background()
	// a slightly different version of the macro defn that supports implicit _doing_ of body (no need for 'do' when multiple expressions in body)
	result, err := REPL(ctx, ns,
		"(do"+
			"(defmacro defn (fn [name params & body] `(def ~name (fn ~params ~@body))))\n"+
			"(defn sum [a b] (str a b) (+ a b))\n"+
			"(sum 1 2))",
		types.NewCursorFile(t.Name()),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.(string) != "3" {
		t.Fatal("test failed")
	}
}
