package lisp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/core"
	"github.com/jig/lisp/reader"
	"github.com/jig/lisp/types"

	. "github.com/jig/lisp/lnotation"
	. "github.com/jig/lisp/types"
)

type Example struct {
	A int
	B string
}

func TestPlaceholders(t *testing.T) {
	repl_env := env.NewEnv()
	core.Load(repl_env)

	str := `(do
				(def v0 $0)
				(def v1 $1)
				(def vNUMBER $NUMBER)
				(def v3 $3)
				(def v4 $4)
				true)`

	exp, err := reader.Read_str(
		str,
		nil,
		&HashMap{
			Val: map[string]MalType{
				"$0":      "hello",
				"$1":      "{\"key\": \"value\"}",
				"$NUMBER": 44,
				"$3":      LS("+", 1, 1),
				"$4": LS("json-encode",
					Example{A: 3, B: "blurp"}),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Println(PRINT(exp))

	ctx := context.Background()
	res, err := EVAL(ctx, exp, repl_env)
	if err != nil {
		t.Fatal(err)
	}
	if res.(bool) {
		v0, err := repl_env.Get(Symbol{Val: "v0"})
		if err != nil {
			t.Fatal(err)
		}
		if v0.(string) != "hello" {
			t.Fatal("no hello")
		}

		v1, err := repl_env.Get(Symbol{Val: "v1"})
		if err != nil {
			t.Fatal(err)
		}
		if v1.(string) != "{\"key\": \"value\"}" {
			t.Fatal("no {\"key\": \"value\"}")
		}

		v2, err := repl_env.Get(Symbol{Val: "vNUMBER"})
		if err != nil {
			t.Fatal(err)
		}
		if v2.(int) != 44 {
			t.Fatal("no 44")
		}

		v3, err := repl_env.Get(Symbol{Val: "v3"})
		if err != nil {
			t.Fatal(err)
		}
		if v3.(int) != 2 {
			t.Fatal("no 2")
		}

		v4, err := repl_env.Get(Symbol{Val: "v4"})
		if err != nil {
			t.Fatal(err)
		}
		switch v4 := v4.(type) {
		case string:
			if v4 != "{\"A\":3,\"B\":\"blurp\"}" {
				t.Fatal("invalid value")
			}
		default:
			t.Fatal("invalid type")
		}
	}
}

func TestREADWithPreamble(t *testing.T) {
	repl_env := env.NewEnv()
	core.Load(repl_env)

	str :=
		`;; $0 "hello"
;; $1 {"key" "value"}
;; $NUMBER 44
;; $4 (+ 1 1)

(do
	(def v0 $0)
	(def v1 $1)
	(def v2 $NUMBER)
	(def v3 $3) ;; this is nil
	(def v4 '$4)
	true)
`

	// exp, err := READ_WithPlaceholders(str, nil, []MalType{"hello", "{\"key\": \"value\"}", 44, List{Val: []MalType{Symbol{Val: "quote"}, List{Val: []MalType{23, 37}}}}})
	exp, err := READWithPreamble(str, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Println(PRINT(exp))
	ctx := context.Background()
	res, err := EVAL(ctx, exp, repl_env)
	if err != nil {
		t.Fatal(err)
	}
	if res.(bool) {
		v0, err := repl_env.Get(Symbol{Val: "v0"})
		if err != nil {
			t.Fatal(err)
		}
		if v0.(string) != "hello" {
			t.Fatal("no hello")
		}

		v1, err := repl_env.Get(Symbol{Val: "v1"})
		if err != nil {
			t.Fatal(err)
		}
		h, ok := v1.(HashMap)
		if !ok {
			t.Fatal("no {\"key\": \"value\"}")
		}
		if len(h.Val) != 1 {
			t.Fatal("pum")
		}
		if h.Val["key"].(string) != "value" {
			t.Fatal("pum2")
		}

		v2, err := repl_env.Get(Symbol{Val: "v2"})
		if err != nil {
			t.Fatal(err)
		}
		if v2.(int) != 44 {
			t.Fatal("no 44")
		}

		v3, err := repl_env.Get(Symbol{Val: "v3"})
		if err != nil {
			t.Fatal(err)
		}
		if v3 != nil {
			t.Fatal("no 2")
		}

		v4, err := repl_env.Get(Symbol{Val: "v4"})
		if err != nil {
			t.Fatal(err)
		}
		l, ok := v4.(List)
		if !ok {
			t.Fatal("no (+ 1 1)")
		}
		if len(l.Val) != 3 {
			t.Fatal("pum3")
		}
		if l.Val[0].(Symbol).Val != "+" {
			t.Fatal("pum4")
		}
		if l.Val[1].(int) != 1 {
			t.Fatal("pum5")
		}
		if l.Val[2].(int) != 1 {
			t.Fatal("pum6")
		}
	}
}

func TestAddPreamble(t *testing.T) {
	repl_env := env.NewEnv()
	core.Load(repl_env)

	str := `(do
	(def v0 $EXAMPLESTRING)
	(def v1 $EXAMPLESTRUCT)
	(def v2 $EXAMPLEINTEGER)
	(def v3 $UNDEFINED) ;; this is nil
	(def v4 '$EXAMPLEAST)
	(def v5 $EXAMPLEBYTESTRING)
	true)`

	eb, err := json.Marshal(Example{A: 1234, B: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := AddPreamble(str, map[string]MalType{
		"$EXAMPLESTRING":  "hello",
		"$EXAMPLESTRUCT":  string(eb),
		"$EXAMPLEINTEGER": 44,
		"$EXAMPLEAST":     LS("+", 1, 1),
		// byte array is handled as string
		"$EXAMPLEBYTESTRING": []byte("byte-array"),
	})
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Println(source)

	exp, err := READWithPreamble(source, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Println(PRINT(exp))

	ctx := context.Background()
	res, err := EVAL(ctx, exp, repl_env)
	if err != nil {
		t.Fatal(err)
	}
	if res.(bool) {
		v0, err := repl_env.Get(Symbol{Val: "v0"})
		if err != nil {
			t.Fatal(err)
		}
		if v0.(string) != "hello" {
			t.Fatal("no hello")
		}

		v1, err := repl_env.Get(Symbol{Val: "v1"})
		if err != nil {
			t.Fatal(err)
		}
		v1Str, ok := v1.(string)
		if !ok {
			t.Fatal("no {\"key\": \"value\"}")
		}
		if v1Str != `{"A":1234,"B":"hello"}` {
			t.Fatal(v1Str)
		}

		v2, err := repl_env.Get(Symbol{Val: "v2"})
		if err != nil {
			t.Fatal(err)
		}
		if v2.(int) != 44 {
			t.Fatal("no 44")
		}

		v3, err := repl_env.Get(Symbol{Val: "v3"})
		if err != nil {
			t.Fatal(err)
		}
		if v3 != nil {
			t.Fatal("no 2")
		}

		v4, err := repl_env.Get(Symbol{Val: "v4"})
		if err != nil {
			t.Fatal(err)
		}
		l, ok := v4.(List)
		if !ok {
			t.Fatal("no (+ 1 1)")
		}
		if len(l.Val) != 3 {
			t.Fatal("pum3")
		}
		if l.Val[0].(Symbol).Val != "+" {
			t.Fatal("pum4")
		}
		if l.Val[1].(int) != 1 {
			t.Fatal("pum5")
		}
		if l.Val[2].(int) != 1 {
			t.Fatal("pum6")
		}
	}
}

func TestAddPreamblePointers(t *testing.T) {
	var1 := 123
	var2 := &var1
	var3 := (*int)(nil)
	for _, tc := range []struct {
		preamble map[string]types.MalType
		expected string
	}{
		{
			preamble: map[string]types.MalType{"$ARG": 123},
			expected: ";; $ARG 123",
		},
		{
			preamble: map[string]types.MalType{"$ARG": var1},
			expected: ";; $ARG 123",
		},
		{
			preamble: map[string]types.MalType{"$ARG": &var1},
			expected: ";; $ARG 123",
		},
		{
			preamble: map[string]types.MalType{"$ARG": var2},
			expected: ";; $ARG 123",
		},
		{
			preamble: map[string]types.MalType{"$ARG": &var2},
			expected: ";; $ARG 123",
		},
		{
			preamble: map[string]types.MalType{"$ARG": var3},
			expected: ";; $ARG nil",
		},
		{
			preamble: map[string]types.MalType{"$ARG": &var3},
			expected: ";; $ARG nil",
		},
		{
			preamble: map[string]types.MalType{"$ARG": nil},
			expected: ";; $ARG nil",
		},
	} {
		expected := fmt.Sprintf("%s\n\n(+ 1 $ARG)", tc.expected)

		sourceCode, err := AddPreamble(`(+ 1 $ARG)`, tc.preamble)
		if err != nil {
			t.Fatal(err)
		}

		if sourceCode != expected {
			t.Fatalf("expected %s, got %s", expected, sourceCode)
		}
	}
}

func TestPlaceholdersEmbeddedWrong1(t *testing.T) {
	repl_env := env.NewEnv()
	core.Load(repl_env)

	str :=
		`$0 "hello"
;; $1 {"key" "value"}
;; $NUMBER 44
;; $4 (+ 1 1)

(do
	(def v0 $0)
	(def v1 $1)
	(def v2 $NUMBER)
	(def v3 $3) ;; this is nil
	(def v4 '$4)
	true)
`

	// exp, err := READ_WithPlaceholders(str, nil, []MalType{"hello", "{\"key\": \"value\"}", 44, List{Val: []MalType{Symbol{Val: "quote"}, List{Val: []MalType{23, 37}}}}})
	_, err := READWithPreamble(str, nil, nil)
	if err == nil {
		t.Fatal("error expected but err was nil")
	}
	if !strings.HasSuffix(err.Error(), "not all tokens where parsed") {
		t.Fatal(err)
	}
}

func TestPlaceholdersEmbeddedNoBlankLine(t *testing.T) {
	repl_env := env.NewEnv()
	core.Load(repl_env)

	// missing blank line must fail
	str :=
		`;; $0 73
;; $1 27
(= (+ $0 $1) 100)
`
	exp, err := READWithPreamble(str, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	res, err := EVAL(ctx, exp, repl_env)
	if err != nil {
		t.Fatal(err)
	}
	if !res.(bool) {
		t.Fatal("failed")
	}
}

var notOptimiseBenchFunc string

func BenchmarkAddPreamble(b *testing.B) {
	for n := 0; n < b.N; n++ {
		var err error
		sourceWithPreamble := `(do
		(def v0 $EXAMPLESTRING)
		(def v2 $EXAMPLEINTEGER)
		(def v3 $UNDEFINED) ;; this is nil
		(def v4 '$EXAMPLEAST)
		(def v5 $EXAMPLEBYTESTRING)
		true)`
		notOptimiseBenchFunc, err = AddPreamble(sourceWithPreamble, map[string]MalType{
			"$EXAMPLESTRING":  "hello",
			"$EXAMPLEINTEGER": 44,
			"$EXAMPLEAST":     LS("+", 1, 1),
			// byte array is handled as string
			"$EXAMPLEBYTESTRING": []byte("byte-array"),
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

var notOptimiseBenchFunc2 MalType

func BenchmarkAddPreambleAlternative(b *testing.B) {
	repl_env := env.NewEnv()
	core.Load(repl_env)

	for n := 0; n < b.N; n++ {
		EXAMPLESTRING := "hello"
		EXAMPLEINTEGER := 44
		EXAMPLEAST := LS("+", 1, 1)
		EXAMPLEBYTESTRING := []byte("byte-array")
		ast := LS("do",
			LS("def", "v0", EXAMPLESTRING),
			LS("def", "v2", EXAMPLEINTEGER),
			LS("def", "v3", nil),
			LS("def", "v4", EXAMPLEAST),
			LS("def", "v5", EXAMPLEBYTESTRING),

			LS("def", LS("not", LS("fn", V([]string{"a"})))),
		)
		notOptimiseBenchFunc2 = PRINT(ast)
	}
}

func BenchmarkREADWithPreamble(b *testing.B) {
	sourceWithPreamble := `(do
		(def v0 $EXAMPLESTRING)
		(def v2 $EXAMPLEINTEGER)
		(def v3 $UNDEFINED) ;; this is nil
		(def v4 '$EXAMPLEAST)
		(def v5 $EXAMPLEBYTESTRING)
		true)`
	codePreamble, err := AddPreamble(sourceWithPreamble, map[string]MalType{
		"$EXAMPLESTRING":  "hello",
		"$EXAMPLEINTEGER": 44,
		"$EXAMPLEAST":     LS("+", 1, 1),
		// byte array is handled as string
		"$EXAMPLEBYTESTRING": []byte("byte-array"),
	})
	if err != nil {
		b.Fatal(err)
	}
	for n := 0; n < b.N; n++ {
		res, err := READWithPreamble(codePreamble, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = res
	}
}

func BenchmarkNewEnv(b *testing.B) {
	repl_env := env.NewEnv()
	core.Load(repl_env)
	sourceWithPreamble := `(do
		(def v0 $EXAMPLESTRING)
		(def v2 $EXAMPLEINTEGER)
		(def v3 $UNDEFINED) ;; this is nil
		(def v4 '$EXAMPLEAST)
		(def v5 $EXAMPLEBYTESTRING)
		true)`
	codePreamble, err := AddPreamble(sourceWithPreamble, map[string]MalType{
		"$EXAMPLESTRING":  "hello",
		"$EXAMPLEINTEGER": 44,
		"$EXAMPLEAST":     LS("+", 1, 1),
		// byte array is handled as string
		"$EXAMPLEBYTESTRING": []byte("byte-array"),
	})
	if err != nil {
		b.Fatal(err)
	}

	ast, err := READWithPreamble(codePreamble, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	for n := 0; n < b.N; n++ {
		res, err := EVAL(ctx, ast, repl_env)
		if err != nil {
			b.Fatal(err)
		}
		if !res.(bool) {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompleteSendingWithPreamble(b *testing.B) {
	for n := 0; n < b.N; n++ {
		source := `(do
			(def v0 $EXAMPLESTRING)
			(def v2 $EXAMPLEINTEGER)
			(def v3 $UNDEFINED) ;; this is nil
			(def v4 '$EXAMPLEAST)
			(def v5 $EXAMPLEBYTESTRING)

			(def not (fn (a) (if a false true)))
			(def b (not $TESTRESULT))
			(not b))`
		sentCode, err := AddPreamble(source, map[string]MalType{
			"$TESTRESULT":     true,
			"$EXAMPLESTRING":  "hello",
			"$EXAMPLEINTEGER": 44,
			"$EXAMPLEAST":     LS("+", 1, 1),
			// byte array is handled as string
			"$EXAMPLEBYTESTRING": []byte("byte-array"),
		})
		if err != nil {
			b.Fatal(err)
		}

		// protocol here

		ast, err := READWithPreamble(sentCode, nil, nil)
		if err != nil {
			b.Fatal(err)
		}

		repl_env := env.NewEnv()
		core.Load(repl_env)
		ctx := context.Background()
		res, err := EVAL(ctx, ast, repl_env)
		if err != nil {
			b.Fatal(err)
		}
		if !res.(bool) {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompleteSendingWithPreambleSolved(b *testing.B) {
	for n := 0; n < b.N; n++ {
		source := `(do
			(def v0 $EXAMPLESTRING)
			(def v2 $EXAMPLEINTEGER)
			(def v3 $UNDEFINED) ;; this is nil
			(def v4 '$EXAMPLEAST)
			(def v5 $EXAMPLEBYTESTRING)

			(def not (fn (a) (if a false true)))
			(def b (not $TESTRESULT))
			(not b))`
		codePreamble, err := AddPreamble(source, map[string]MalType{
			"$TESTRESULT":     true,
			"$EXAMPLESTRING":  "hello",
			"$EXAMPLEINTEGER": 44,
			"$EXAMPLEAST":     LS("+", 1, 1),
			// byte array is handled as string
			"$EXAMPLEBYTESTRING": []byte("byte-array"),
		})
		if err != nil {
			b.Fatal(err)
		}

		sentAST, err := READWithPreamble(codePreamble, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		sentCode := PRINT(sentAST)

		// protocol here

		repl_env := env.NewEnv()
		core.Load(repl_env)

		ast, err := READ(sentCode, nil, repl_env)
		if err != nil {
			b.Fatal(err)
		}

		ctx := context.Background()
		res, err := EVAL(ctx, ast, repl_env)
		if err != nil {
			b.Fatal(err)
		}
		if !res.(bool) {
			b.Fatal(err)
		}
	}
}

func TestHashMapMarshalers(t *testing.T) {
	repl_env := env.NewEnv()
	core.Load(repl_env)

	str := `(do
				(def go-struct $GOSTRUCT)
				true)`

	source, err := AddPreamble(str, map[string]MalType{
		"$GOSTRUCT": LispMarshalExample{MarshalExample{A: 1984, B: "I am B"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Println(source)

	exp, err := READWithPreamble(source, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// fmt.Println(PRINT(exp))
	ctx := context.Background()
	res, err := EVAL(ctx, exp, repl_env)
	if err != nil {
		t.Fatal(err)
	}
	if res.(bool) {
		goStruct, err := repl_env.Get(Symbol{Val: "go-struct"})
		if err != nil {
			t.Fatal(err)
		}
		if goStruct.(HashMap).Val["ʞa"] != 1984 {
			t.Fatal("no 1984")
		}
		if goStruct.(HashMap).Val["ʞb"] != "I am B" {
			t.Fatal("no B")
		}
	}
}

func TestPassingLispDataFromGo(t *testing.T) {
	m := map[string]interface{}{
		"a": 1,
		"b": 2,
		"c": map[string]interface{}{
			"d": 4,
		},
	}
	v := []string{"hello", "world"}
	vs := []MarshalExample{
		{A: 0, B: "hello"},
		{A: 1, B: "world"},
	}
	source := `(do
					(def hm $HM)
					(def l $L)
					(def v $V)
					(def s $S)
					(def vs $VS)
					(assert (= 2 l))
					(assert (contains? s "bob"))
					(assert (= "hello" (get v 0)))
					;; (assert (= "hello" (get-in vs [1 "b"])))
					true)`
	sentCode, err := AddPreamble(source, map[string]MalType{
		"$HM": HM(m),
		"$L":  LS("+", 1, 1),
		"$V":  V(v),
		"$S":  SET([]string{"alice", "bob", "charly"}),
		"$VS": V(vs),
	})
	if err != nil {
		t.Fatal(err)
	}

	// protocol here

	ast, err := READWithPreamble(sentCode, NewCursorFile(t.Name()), nil)
	if err != nil {
		t.Fatal(err)
	}

	ns := env.NewEnv()
	core.Load(ns)
	ctx := context.Background()
	res, err := EVAL(ctx, ast, ns)
	if err != nil {
		t.Fatal(err)
	}
	if !res.(bool) {
		t.Fatal(err)
	}
}
