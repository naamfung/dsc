package lisp

import (
	"context"
	"testing"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/concurrent"
	"github.com/jig/lisp/lib/core"
	"github.com/jig/lisp/types"
)

func TestCursor(t *testing.T) {
	bootEnv := env.NewEnv()
	core.Load(bootEnv)
	core.LoadInput(bootEnv)
	concurrent.Load(bootEnv)

	bootEnv.Set(types.Symbol{Val: "eval"}, types.Func{Fn: func(ctx context.Context, a []types.MalType) (types.MalType, error) {
		return EVAL(ctx, a[0], bootEnv)
	}})
	bootEnv.Set(types.Symbol{Val: "*ARGV*"}, types.List{})

	ctx := context.Background()
	// core.mal: defined using the language itself
	_, err := REPL(ctx, bootEnv, `(def *host-language* "go")`, types.NewCursorFile(t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		Module string
		Code   string
		Cursor *types.Position
	}{
		{
			Module: "nested",
			Code:   nested,
			Cursor: types.NewAnonymousCursorHere(1, 24),
		},
		{
			Module: "singleline-string",
			Code:   singleline,
			Cursor: types.NewAnonymousCursorHere(1, 6),
		}, {
			Module: "multiline-string",
			Code:   multiline,
			Cursor: types.NewAnonymousCursorHere(6, 2),
		}, {
			Module: "codeThrow",
			Code:   codeThrow,
			Cursor: types.NewAnonymousCursorHere(4, 2),
		},
		{
			Module: "codeTryAndThrowAndCatch",
			Code:   codeTryAndThrowAndCatch,
			Cursor: nil,
		},
		{
			Module: "codeUndefinedSymbol",
			Code:   codeUndefinedSymbol,
			Cursor: types.NewAnonymousCursorHere(3, 17),
		},
		{
			Module: "codeLetIsBogus",
			Code:   codeLetIsBogus,
			Cursor: types.NewAnonymousCursorHere(3, 7),
		},
		{
			Module: "codeCorrect",
			Code:   codeCorrect,
			Cursor: nil,
		},
		{
			Module: "codeMissingRightBracket",
			Code:   codeMissingRightBracket,
			Cursor: types.NewAnonymousCursorHere(8, 2),
		},
		{
			Module: "codeTooManyRightBrackets",
			Code:   codeTooManyRightBrackets,
			Cursor: types.NewAnonymousCursorHere(8, 28),
		},
	} {
		subEnv := env.NewSubordinateEnv(bootEnv)
		_, err := REPL(ctx, subEnv, "(do "+testCase.Code+")", &types.Position{
			Module: &testCase.Module,
			Row:    0,
		})
		switch err := err.(type) {
		case nil:
			if testCase.Cursor != nil {
				t.Fatalf("Expected error %q", testCase.Cursor)
			}
			continue
		case interface {
			Position() *types.Position
			Error() string
		}:
			if err.Position() == nil {
				t.Fatal("error")
			}
			if !err.Position().Includes(*testCase.Cursor) {
				t.Errorf("%s != %s", err.Position(), testCase.Cursor)
			}
			continue
		default:
			t.Fatal(err)
		}
	}
}

var singleline = `(throw "pum")`

var nested = `(def fpum (fn [x] (throw x)))
(def f1 (fn [x] x))
(def f2 (fn [x] x))
(def f3 (fn [x] x))
(f1 (f2 (f3 (fpum "pum"))))`

var multiline = `;; multiline strings

(def multi ¬line1
	line6¬)

(throw "pum")`

var codeCorrect = `;; prerequisites
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

(def a 1234)
`

var codeMissingRightBracket = `;; prerequisites
;; Trivial but convenient functions.

;; Integer predecessor (number -> number)
(def inc (fn [a] (+ a 1)))

;; Integer predecessor (number -> number) ;; MISSING ) ON NEXT LINE:
(def dec (fn (a) (- a 1))

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

(def a 1234)
`

var codeTooManyRightBrackets = `;; prerequisites
;; Trivial but convenient functions.

;; Integer predecessor (number -> number)
(def inc (fn [a] (+ a 1)))

;; Integer predecessor (number -> number)
(def dec (fn (a) (- a 1))))

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

(def a 1234)
`
var codeThrow = `;; this will throw an error
;; in a trivial way

(throw "boo")
`

var codeTryAndThrowAndCatch = `;; throwing an error and catching
;; must not involve program lines

(try
	abc
	(catch exc
		(str "exc is:" exc)))

(def a 1234)
`

// var codeTryAndThrow = `;; throwing an error and catching
// ;; must not involve program lines

// (try
// 	abc
// 	(catch exc
// 		(str "exc is:" exc)))

// (def a 1234)
// `

var codeLetIsBogus = `;; let requires a vector with even elements

(let [x 1
	y]
	y)
`

var codeUndefinedSymbol = `;; undefined-symbol is undefined

undefined-symbol
`
