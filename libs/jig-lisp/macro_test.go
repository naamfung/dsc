package lisp

import (
	"context"
	_ "embed"
	"reflect"
	"testing"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/concurrent"
	"github.com/jig/lisp/lib/core"
	"github.com/jig/lisp/lib/coreextented"
	"github.com/jig/lisp/types"
)

//go:embed macro_test.lisp
var macro_test string

func Load(repl_env types.EnvType) error {
	ctx := context.Background()
	for _, symbols := range []string{
		trivial,
		benchmark,
		reducers,
		threading,
		// equality,
		memoize,
		perf,
		pprint,
		protocols,
		test_cascade,
		load_file_once,
	} {
		if _, err := REPL(ctx, repl_env, `(eval (read-string (str "(do "`+symbols+`" nil)")))`, types.NewCursorFile(reflect.TypeOf(_here_{}).PkgPath())); err != nil {
			return err
		}
	}
	return nil
}

var trivial = `;; prerequisites
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
`

var benchmark = `;; An alternative approach, to complement perf.mal
;; requires Trivial

(def benchmark* (fn [f n results]
  (if (< 0 n)
    (let [start-ms (time-ms)
           _ (f)
           end-ms (time-ms)]
      (benchmark* f (- n 1) (conj results (- end-ms start-ms))))
    results)))

(defmacro benchmark (fn [expr n]
  ` + "`" + `(benchmark* (fn [] ~expr) ~n [])))
`

var reducers = `;; Left and right folds.

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
`

var threading = `;; Composition of partially applied functions.

;; Rewrite x (a a1 a2) .. (b b1 b2) as
;;   (b (.. (a x a1 a2) ..) b1 b2)
;; If anything else than a list is found were "(a a1 a2)" is expected,
;; replace it with a list with one element, so that "-> x a" is
;; equivalent to "-> x (list a)".
(defmacro ->
  (fn (x & xs)
    (reduce _iter-> x xs)))

(def _iter->
  (fn [acc form]
    (if (list? form)
      ` + "`" + `(~(first form) ~acc ~@(rest form))
      (list form acc))))

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
      (list form acc))))
`

// var equality = `;; equality.mal

// ;; This file checks whether the "=" function correctly implements equality of
// ;; hash-maps and sequences (lists and vectors).  If not, it redefines the "="
// ;; function with a pure mal (recursive) implementation that only relies on the
// ;; native original "=" function for comparing scalars (integers, booleans,
// ;; symbols, strings, keywords, atoms, nil).

// ;; Save the original (native) "=" as scalar-equal?
// (def scalar-equal? =)

// ;; A faster "and" macro which doesn't use "=" internally.
// (defmacro bool-and                    ; boolean
//   (fn [& xs]                          ; interpreted as logical values
//     (if (empty? xs)
//       true
//       ` + "`" + `(if ~(first xs) (bool-and ~@(rest xs)) false))))

// (defmacro bool-or                     ; boolean
//   (fn [& xs]                          ; interpreted as logical values
//     (if (empty? xs)
//       false
//       ` + "`" + `(if ~(first xs) true (bool-or ~@(rest xs))))))

// (def starts-with?
//   (fn [a b]
//     (bool-or (empty? a)
//              (bool-and (mal-equal? (first a) (first b))
//                        (starts-with? (rest a) (rest b))))))

// (def hash-map-vals-equal?
//   (fn [a b map-keys]
//     (bool-or (empty? map-keys)
//              (let [key (first map-keys)]
//                (bool-and (contains? b key)
//                          (mal-equal? (get a key) (get b key))
//                          (hash-map-vals-equal? a b (rest map-keys)))))))

// ;; This implements = in pure mal (using only scalar-equal? as native impl)
// (def mal-equal?
//   (fn [a b]
//     (cond

//       (sequential? a)
//       (bool-and (sequential? b)
//                 (scalar-equal? (count a) (count b))
//                 (starts-with? a b))

//       (map? a)
//       (let [keys-a (keys a)]
//         (bool-and (map? b)
//                   (scalar-equal? (count keys-a) (count (keys b)))
//                   (hash-map-vals-equal? a b keys-a)))

//       true
//       (scalar-equal? a b))))

// (def hash-map-equality-correct?
//   (fn []
//     (try
//       (bool-and (= {:a 1} {:a 1})
//                 (not (= {:a 1} {:a 1 :b 2})))
//       (catch _ false))))

// (def sequence-equality-correct?
//   (fn []
//     (try
//       (bool-and (= [:a :b] (list :a :b))
//                 (not (= [:a :b] [:a :b :c])))
//       (catch _ false))))

// ;; If the native "=" implementation doesn't support sequences or hash-maps
// ;; correctly, replace it with the pure mal implementation
// (if (not (bool-and (hash-map-equality-correct?)
//                    (sequence-equality-correct?)))
//   (do
//     (def = mal-equal?)
//     (println "equality.mal: Replaced = with pure mal implementation")))
// `

var memoize = `;; Memoize any function.

;; Implement "memoize" using an atom ("mem") which holds the memoized results
;; (hash-map from the arguments to the result). When the function is called,
;; the hash-map is checked to see if the result for the given argument was already
;; calculated and stored. If this is the case, it is returned immediately;
;; otherwise, it is calculated and stored in "mem".

;; For recursive functions, take care to store the wrapper under the
;; same name than the original computation with an assignment like
;; "(def f (memoize f))", so that intermediate results are memorized.

;; Adapted from http://clojure.org/atoms

(def memoize
  (fn [f]
    (let [mem (atom {})]
      (fn [& args]
        (let [key (str args)]
          (if (contains? @mem key)
            (get @mem key)
            (let [ret (apply f args)]
              (do
                (swap! mem assoc key ret)
                ret))))))))
`

var perf = `;; Mesure performances.
;; requires trivial package

;; Evaluate an expression, but report the time spent
(defmacro time
  (fn (exp)
    (let [start (gensym)
           ret   (gensym)]
      ` + "`" + `(let (~start (time-ms)
              ~ret   ~exp)
        (do
          (println "Elapsed time:" (- (time-ms) ~start) "msecs")
          ~ret)))))

;; Count evaluations of a function during a given time frame.
(def run-fn-for

  (let [
    run-fn-for* (fn [fn max-ms acc-ms last-iters]
      (let [start (time-ms)
             _ (fn)
             elapsed (- (time-ms) start)
             iters (inc last-iters)
             new-acc-ms (+ acc-ms elapsed)]
        ;; (do (prn "new-acc-ms:" new-acc-ms "iters:" iters))
        (if (>= new-acc-ms max-ms)
          last-iters
          (run-fn-for* fn max-ms new-acc-ms iters))))
    ]

    (fn [fn max-secs]
      ;; fn       : function without parameters
      ;; max-secs : number (seconds)
      ;; return   : number (iterations)
      (do
        ;; Warm it up first
        (run-fn-for* fn 1000 0 0)
        ;; Now do the test
        (run-fn-for* fn (* 1000 max-secs) 0 0)))))
`

var pprint = `;; Pretty printer a MAL object.

(def pprint

  (let [

    spaces- (fn [indent]
      (if (> indent 0)
        (str " " (spaces- (- indent 1)))
        ""))

    pp-seq- (fn [obj indent]
      (let [xindent (+ 1 indent)]
        (apply str (pp- (first obj) 0)
                   (map (fn [x] (str "\n" (spaces- xindent)
                                      (pp- x xindent)))
                        (rest obj)))))

    pp-map- (fn [obj indent]
      (let [ks (keys obj)
             kindent (+ 1 indent)
             kwidth (count (seq (str (first ks))))
             vindent (+ 1 (+ kwidth kindent))]
        (apply str (pp- (first ks) 0)
                   " "
                   (pp- (get obj (first ks)) 0)
                   (map (fn [k] (str "\n" (spaces- kindent)
                                      (pp- k kindent)
                                      " "
                                      (pp- (get obj k) vindent)))
                        (rest ks)))))

    pp- (fn [obj indent]
      (cond
        (list? obj)   (str "(" (pp-seq- obj indent) ")")
        (vector? obj) (str "[" (pp-seq- obj indent) "]")
        (map? obj)    (str "{" (pp-map- obj indent) "}")
        :else         (pr-str obj)))

    ]

    (fn [obj]
         (println (pp- obj 0)))))
`

var protocols = `;; A sketch of Clojure-like protocols, implemented in Mal

;; By chouser (Chris Houser)
;; Original: https://gist.github.com/Chouser/6081ea66d144d13e56fc

;; This function maps a MAL value to a keyword representing its type.
;; Most applications will override the default with an explicit value
;; for the ":type" key in the metadata.
(def find-type (fn [obj]
  (cond
    (symbol?  obj) :mal/symbol
    (keyword? obj) :mal/keyword
    (atom?    obj) :mal/atom
    (nil?     obj) :mal/nil
    (true?    obj) :mal/boolean
    (false?   obj) :mal/boolean
    (number?  obj) :mal/number
    (string?  obj) :mal/string
    (macro?   obj) :mal/macro
    true
    (let [metadata (meta obj)
           type     (if (map? metadata) (get metadata :type))]
      (cond
        (keyword? type) type
        (list?   obj)   :mal/list
        (vector? obj)   :mal/vector
        (map?    obj)   :mal/map
        (fn?     obj)   :mal/function
        true            (throw "unknown MAL value in protocols"))))))

;; A protocol (abstract class, interface..) is represented by a symbol.
;; It describes methods (abstract functions, contracts, signals..).
;; Each method is described by a sequence of two elements.
;; First, a symbol setting the name of the method.
;; Second, a vector setting its formal parameters.
;; The first parameter is required, plays a special role.
;; It is usually named "this" ("self"..).
;; For example,
;;   (defprotocol protocol
;;     (method1 [this])
;;     (method2 [this argument]))
;; can be thought as:
;;   (def method1 (fn [this]) ..)
;;   (def method2 (fn [this argument]) ..)
;;   (def protocol ..)
;; The return value is the new protocol.
(defmacro defprotocol (fn [proto-name & methods]
  ;; A protocol is an atom mapping a type extending the protocol to
  ;; another map from method names as keywords to implementations.
  (let [
    drop2 (fn [args]
      (if (= 2 (count args))
        ()
        (cons (first args) (drop2 (rest args)))))
    rewrite (fn [method]
      (let [
        name     (first method)
        args     (nth method 1)
        argc     (count args)
        varargs? (if (<= 2 argc) (= '& (nth args (- argc 2))))
        dispatch ` + "`" + `(get (get @~proto-name
                            (find-type ~(first args)))
                       ~(keyword (str name)))
        body     (if varargs?
          ` + "`" + `(apply ~dispatch ~@(drop2 args) ~(nth args (- argc 1)))
                   (cons dispatch args))
        ]
        (list 'def name (list 'fn args body))))
    ]
      ` + "`" + `(do
      ~@(map rewrite methods)
       (def ~proto-name (atom {}))))))

;; A type (concrete class..) extends (is a subclass of, implements..)
;; a protocol when it provides implementations for the required methods.
;;   (extend type protocol {
;;     :method1 (fn [this] ..)
;;     :method2 (fn [this arg1 arg2])})
;; Additionnal protocol/methods pairs are equivalent to successive
;; calls with the same type.
;; The return value is "nil".
(def extend (fn [type proto methods & more]
  (do
    (swap! proto assoc type methods)
    (if (first more)
      (apply extend type more)))))

;; An object satisfies a protocol when its type extends the protocol,
;; that is if the required methods can be applied to the object.
(def satisfies? (fn [protocol obj]
  (contains? @protocol (find-type obj))))
;; If "(satisfies protocol obj)" with the protocol below
;; then "(method1 obj)" and "(method2 obj 1 2)"
;; dispatch to the concrete implementation provided by the exact type.
;; Should the type evolve, the calling code needs not change.
`

var test_cascade = `;; Iteration on evaluations interpreted as boolean values.

;; "(cond test1 result1 test2 result2 .. testn resultn)"
;; is rewritten (in the step files) as
;; "(if test1 result1 (if test2 result2 (.. (if testn resultn nil))))"
;; It is common that "testn" is ""else"", ":else", "true" or similar.

;; "(or x1 x2 .. xn x)"
;; is almost rewritten as
;; "(if x1 x1 (if x2 x2 (.. (if xn xn x))))"
;; except that each argument is evaluated at most once.
;; Without arguments, returns "nil".
(defmacro or (fn [& xs]
  (if (< (count xs) 2)
    (first xs)
    (let [r (gensym)]
      ` + "`" + `(let (~r ~(first xs)) (if ~r ~r (or ~@(rest xs))))))))

;; Conjonction of predicate values (pred x1) and .. and (pred xn)
;; Evaluate "pred x" for each "x" in turn. Return "false" if a result
;; is "nil" or "false", without evaluating the predicate for the
;; remaining elements.  If all test pass, return "true".
(def every?
  (fn (pred xs)
    ;; pred   : Element -> interpreted as a logical value
    ;; xs     : sequence of Elements x1 x2 .. xn
    ;; return : boolean
    (cond (empty? xs)       true
          (pred (first xs)) (every? pred (rest xs))
          true              false)))

;; Disjonction of predicate values (pred x1) or .. (pred xn)
;; Evaluate "(pred x)" for each "x" in turn. Return the first result
;; that is neither "nil" nor "false", without evaluating the predicate
;; for the remaining elements.  If all tests fail, return nil.
(def some
  (fn (pred xs)
    ;; pred   : Element -> interpreted as a logical value
    ;; xs     : sequence of Elements x1 x2 .. xn
    ;; return : boolean
    (if (empty? xs)
      nil
      (or (pred (first xs))
          (some pred (rest xs))))))

;; Search for first evaluation returning "nil" or "false".
;; Rewrite "x1 x2 .. xn x" as
;;   (let [r1 x1]
;;     (if r1 test1
;;       (let [r2 x2]
;;         ..
;;         (if rn
;;           x
;;           rn) ..)
;;       r1))
;; Without arguments, returns "true".
(defmacro and
  (fn (& xs)
    ;; Arguments and the result are interpreted as boolean values.
    (cond (empty? xs)      true
          (= 1 (count xs)) (first xs)
          true             (let (condvar (gensym))
                             ` + "`" + `(let (~condvar ~(first xs))
                               (if ~condvar (and ~@(rest xs)) ~condvar))))))
`

var load_file_once = `;; Like load-file, but will never load the same path twice.

;; This file is normally loaded with "load-file", so it needs a
;; different mechanism to neutralize multiple inclusions of
;; itself. Moreover, the file list should never be reset.

(def load-file-once
  (try
    load-file-once
  (catch _
    (let [seen (atom {"../lib/load-file-once.mal" nil})]
      (fn [filename]
        (if (not (contains? @seen filename))
          (do
            (swap! seen assoc filename nil)
            (load-file filename))))))))
`

type _here_ struct{}

func TestMacro(t *testing.T) {
	repl_env := env.NewEnv()
	ctx := context.Background()

	core.Load(repl_env)
	core.LoadInput(repl_env)
	concurrent.Load(repl_env)

	repl_env.Set(types.Symbol{Val: "eval"}, types.Func{Fn: func(ctx context.Context, a []types.MalType) (types.MalType, error) {
		return EVAL(ctx, a[0], repl_env)
	}})

	if _, err := REPL(ctx, repl_env, core.HeaderBasic(), types.NewCursorFile(reflect.TypeOf(_here_{}).PkgPath())); err != nil {
		t.Fatal(err)
	}
	if _, err := REPL(ctx, repl_env, core.HeaderLoadFile(), types.NewCursorFile(reflect.TypeOf(_here_{}).PkgPath())); err != nil {
		t.Fatal(err)
	}
	if _, err := REPL(ctx, repl_env, coreextented.HeaderCoreExtended(), types.NewCursorFile(reflect.TypeOf(_here_{}).PkgPath())); err != nil {
		t.Fatal(err)
	}
	if _, err := REPL(ctx, repl_env, concurrent.HeaderConcurrent(), types.NewCursorFile(reflect.TypeOf(_here_{}).PkgPath())); err != nil {
		t.Fatal(err)
	}

	exp, err := READ(macro_test, nil, repl_env)
	if err != nil {
		t.Fatal(err)
	}
	exp, err = EVAL(ctx, exp, repl_env)
	if err != nil {
		t.Fatal(err)
	}
	res := PRINT(exp)

	if res != "true" {
		t.Fatal(res)
	}
}
