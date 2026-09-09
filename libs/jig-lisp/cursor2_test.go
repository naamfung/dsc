package lisp

import (
	"context"
	"testing"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/concurrent"
	"github.com/jig/lisp/lib/core"
	"github.com/jig/lisp/types"
)

func TestCursor2(t *testing.T) {
	bootEnv := env.NewEnv()
	core.Load(bootEnv)
	core.LoadInput(bootEnv)
	concurrent.Load(bootEnv)

	bootEnv.Set(types.Symbol{Val: "eval"}, types.Func{Fn: func(ctx context.Context, a []types.MalType) (types.MalType, error) {
		return EVAL(ctx, a[0], bootEnv)
	}})
	bootEnv.Set(types.Symbol{Val: "*ARGV*"}, types.List{})

	ctx := context.Background()
	_, err := REPL(ctx, bootEnv, codeMacro, types.NewCursorFile(t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = REPL(ctx, bootEnv, testCode, types.NewCursorFile(t.Name()))
	switch err := err.(type) {
	case nil:
		t.Error("unexpected: no error returned")
	case interface{ GetPosition() *types.Position }:
		if err.GetPosition().Row != 11 {
			t.Fatalf("%+v %s", err.GetPosition(), err)
		}
	}
}

const codeMacro = `(do
;; prerequisites
;; Trivial but convenient functions.

;; Integer predecessor (number -> number)
(def inc (fn [a] (+ a 1)))

;; Integer predecessor (number -> number)
(def dec (fn (a) (- a 1)))

;; Integer nullity test (number -> boolean)
(def zero? (fn (n) (= 0 n)))

;; Returns the unchanged argument.
(def identity (fn (x) x))

;; Generate a hopefully unique symbol. See section "Plugging the Leaks"
;; of http://www.gigamonkeys.com/book/macros-defining-your-own.html
(def gensym
	(let [counter (atom 0)]
	(fn []
		(symbol (str "G__" (swap! counter inc))))))
;; Left and right folds.

;; Left fold (f (.. (f (f init x1) x2) ..) xn)
(def reduce
	(fn (f init xs)
	;; f      : Accumulator Element -> Accumulator
	;; init   : Accumulator
	;; xs     : sequence of Elements x1 x2 .. xn
	;; return : Accumulator
	(if (empty? xs)
		init
		(reduce f (f init (first xs)) (rest xs)))))

;; Left fold for maps (f (.. (f (f init x1) x2) ..) xn)
(def reduce-kv
	(fn [f init xs]
	;; f      : Accumulator Element -> Accumulator
	;; init   : Accumulator
	;; xs     : sequence of key-value pairs k1-v1 k2-v2...
	;; return : Accumulator
	(if (empty? xs)
		init
		(reduce-kv f (f init (nth xs 0) (nth xs 1)) (rest (rest xs))))))

;; Right fold (f x1 (f x2 (.. (f xn init)) ..))
;; The natural implementation for 'foldr' is not tail-recursive, and
;; the one based on 'reduce' constructs many intermediate functions, so we
;; rely on efficient 'nth' and 'count'.
(def foldr
	(let [
	rec (fn [f xs acc index]
		(if (< index 0)
		acc
		(rec f xs (f (nth xs index) acc) (- index 1))))
	]

	(fn [f init xs]
		;; f      : Element Accumulator -> Accumulator
		;; init   : Accumulator
		;; xs     : sequence of Elements x1 x2 .. xn
		;; return : Accumulator
		(rec f xs init (- (count xs) 1)))))

		;; Composition of partially applied functions.
(def _iter->
	(fn [acc form]
	(if (list? form)
		` + "`" + `(~(first form) ~acc ~@(rest form))
		(list form acc))))

;; Rewrite x (a a1 a2) .. (b b1 b2) as
;;   (b (.. (a x a1 a2) ..) b1 b2)
;; If anything else than a list is found were "(a a1 a2)" is expected,
;; replace it with a list with one element, so that "-> x a" is
;; equivalent to "-> x (list a)".
(defmacro ->
	(fn (x & xs)
	(reduce _iter-> x xs)))

;; Like "->", but the arguments describe functions that are partially
;; applied with *left* arguments.  The previous result is inserted at
;; the *end* of the new argument list.
;; Rewrite x ((a a1 a2) .. (b b1 b2)) as
;;   (b b1 b2 (.. (a a1 a2 x) ..)).
(defmacro ->>
	(fn (x & xs)
		(reduce _iter->> x xs)))

(def _iter->>
	(fn [acc form]
	(if (list? form)
	` + "`" + `(~(first form) ~@(rest form) ~acc)
		(list form acc)))))
`

const testCode = `(println
	(-> {}
		(assoc :a "a")
		(assoc :b "b")
		(assoc :b "b")
		(assoc :b "b")
		(assoc :b "b")
		(assoc :b "b")
		(assoc :b "b")
		(assoc :b "b")
		(assoc :b "b")
		(assoc :c (/ 0 0))
		(assoc :d "d")))`
