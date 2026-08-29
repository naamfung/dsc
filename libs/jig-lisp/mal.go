// Copyright (C) 2015 Joel Martin <github@martintribe.org>
// This code is derived from MAL (Make-A-Lisp) by Joel Martin and follows same licence.
// license that can be found in the LICENSE file.

// Copyright 2022 Jordi Íñigo Griera. All rights reserved.

// Package lisp provides a minimal Lisp interpreter focused on embedded work in
// Go code, as config or as a transmission format.
// Lisp external libraries are loaded from the Go code, and loading them from Lisp code is
// not allowed (on purpose).
//
// This interpreter is based on [kanaka/mal] implementation that is inspired on Clojure.
// It is still mostly compatible with kanaka/mal except that def!, try*, etc. symbols
// have been changed to def, try, etc. See ./examples/mal.lisp as a port of mal.mal
//
// Overview of this implementation addition to kanaka/mal:
//   - simpler embedded use with a simple package API (mostly inherited, just code reorganisation)
//   - testing based on Go tooling (all python tests scripts substituted by Go tests, see ./run_test.go)
//   - support of Go constructors to simplify extendability
//   - slightly faster parsing by swapping regex implementation for a text/scanner one
//   - support of preamble (AKA "placeholders") to simplify parametrisation of Go functions implemented on Lisp
//   - easier library development (using reflect)
//   - line numbers
//
// Functions and file directories keep the same structure as original MAL, this is way
// main functions [READ], [EVAL] and [PRINT] keep its all caps (non Go standard) names.
//
// [kanaka/mal]: https://github.com/kanaka/mal
package lisp

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	. "github.com/jig/lisp/env"
	"github.com/jig/lisp/lisperror"
	"github.com/jig/lisp/printer"
	"github.com/jig/lisp/reader"
	. "github.com/jig/lisp/types"
)

var placeholderRE = regexp.MustCompile(`^(;; \$[\-\d\w]+)+\s(.+)`)

const preamblePrefix = ";; $"

// DebugEvalEnabled controls whether DEBUG-EVAL support is active.
// When false, the DEBUG-EVAL check is skipped entirely for better performance.
// Set this to true before evaluation if you need DEBUG-EVAL functionality.
var DebugEvalEnabled = false

// READ reads Lisp source code and generates an AST that might be evaled by [EVAL] or printed by [PRINT].
//
// cursor and environment might be passed nil and READ will provide correct values for you.
// It is recommended though that cursor is initialised with a source code file identifier to
// provide better positioning information in case of encountering an execution error.
//
// EnvType is required in case you expect to parse Go constructors
func READ(sourceCode string, cursor *Position, ns EnvType) (MalType, error) {
	return reader.Read_str(sourceCode, cursor, nil, ns)
}

// READWithPreamble reads Lisp source code with preamble placeholders and generates an
// AST that might be evaled by [EVAL] or printed by [PRINT].
//
// cursor and environment might be passed nil and READ will provide correct values for you.
// It is recommended though that cursor is initialised with a source code file identifier to
// provide better positioning information in case of encountering an execution error.
//
// # EnvType is required in case you expect to parse Go constructors.
//
// Preamble placeholders are prefix the source code and have the following format:
//
//	;; <$-prefixed-var-name> <Lisp readable expression>
//
// For example:
//
//	;; $1 {:key "value"}
//	;; $NUMBER 1984
//	;; $EXPR1 (+ 1 1)
//
// will create three values that will fill the placeholders in the source code. Following the example
// the source code might look like:
//
//	...some code...
//	(prn "$NUMBER is" $NUMBER)
//
// note that the actual code to be parsed will be:
//
//	(prn "$NUMBER is" 1984)
//
// this simplifies inserting Lisp code in Go packages and passing Go parameters to it.
//
// Look for the "L-notation" to simplify the pass of complex Lisp structures as placeholders.
//
// READWithPreamble is used to read code (actually decode) on transmission. Use [AddPreamble]
// when calling from Go code.
func READWithPreamble(str string, cursor *Position, ns EnvType) (MalType, error) {
	placeholderMap := &HashMap{Val: map[string]MalType{}}
	i := 0
	for ; ; i++ {
		var line string
		// line, str, _ = strings.Cut(str, "\n")
		line, str, _ = strings.Cut(str, "\n")
		line = strings.Trim(line, " \t\r\n")
		if len(line) == 0 {
			return reader.Read_str(str, cursor, placeholderMap, ns)
		}
		if !strings.HasPrefix(line, preamblePrefix) {
			return reader.Read_str(line+"\n"+str, cursor, placeholderMap, ns)
		}
		lineItems := placeholderRE.FindAllStringSubmatch(line, -1)
		if len(lineItems) != 1 || len(lineItems[0]) != 3 {
			return nil, lisperror.NewLispError(errors.New("invalid preamble format"), &Position{
				Row: i + 1,
				Col: 1,
			})
		}
		placeholderValue := lineItems[0][2]
		item, _ := reader.Read_str(placeholderValue, &Position{
			Row: i + 1,
			Col: 1,
		}, nil, ns)
		placeholderKey := lineItems[0][1][3:]
		placeholderMap.Val[placeholderKey] = item
	}
}

// AddPreamble combines prefix variables into a preamble to the provided source code.
//
// Source code encoded be readed with [READWithPreamble].
// placeholderMap must contain a map with keys being the variable names on the placeholder and the
// values the AST assigned to each placeholder. Value ASTs might be generated with [READ] or [EVAL] or with the
// [lnotation] package (most likely). Key names must contain the '$' prefix.
func AddPreamble(str string, placeholderMap map[string]MalType) (string, error) {
	preamble := ""
	for placeholderKey, placeholderValue := range placeholderMap {
		preamble = preamble + ";; " + placeholderKey + " " + PRINT(placeholderValue) + "\n"
	}
	return preamble + "\n" + str, nil
}

func starts_with(xs []MalType, sym string) bool {
	if 0 < len(xs) {
		switch s := xs[0].(type) {
		case Symbol:
			return s.Val == sym
		default:
		}
	}
	return false
}

func qq_loop(xs []MalType) MalType {
	acc := NewList(nil)
	for i := len(xs) - 1; 0 <= i; i -= 1 {
		elt := xs[i]
		switch e := elt.(type) {
		case List:
			if starts_with(e.Val, "splice-unquote") {
				acc = NewList(lisperror.GetPosition(elt), Symbol{Val: "concat"}, e.Val[1], acc)
				continue
			}
		default:
		}
		acc = NewList(lisperror.GetPosition(elt), Symbol{Val: "cons"}, quasiquote(elt), acc)
	}
	return acc
}

func quasiquote(ast MalType) MalType {
	switch a := ast.(type) {
	case Vector:
		return NewList(a.Cursor, Symbol{Val: "vec"}, qq_loop(a.Val))
	case HashMap, Symbol:
		return NewList(lisperror.GetPosition(ast), Symbol{Val: "quote"}, ast)
	case List:
		if starts_with(a.Val, "unquote") {
			return a.Val[1]
		} else {
			return qq_loop(a.Val)
		}
	default:
		return ast
	}
}

func is_macro_call(ast MalType, env EnvType) bool {
	if Q[List](ast) {
		slc, _ := GetSlice(ast)
		if len(slc) == 0 {
			return false
		}
		a0 := slc[0]
		if Q[Symbol](a0) && env.Find(a0.(Symbol)) != nil {
			mac, e := env.Get(a0.(Symbol))
			if e != nil {
				return false
			}
			if Q[MalFunc](mac) {
				return mac.(MalFunc).GetMacro()
			}
		}
	}
	return false
}

func macroexpand(ctx context.Context, ast MalType, env EnvType) (MalType, error) {
	var mac MalType
	var e error
	for is_macro_call(ast, env) {
		slc, _ := GetSlice(ast)
		a0 := slc[0]
		mac, e = env.Get(a0.(Symbol))
		if e != nil {
			return nil, e
		}
		fn := mac.(MalFunc)
		ast, e = Apply(ctx, fn, slc[1:])
		if e != nil {
			return nil, e
		}
	}
	return ast, nil
}

func eval_ast(ctx context.Context, ast MalType, env EnvType) (MalType, error) {
	if Q[Symbol](ast) {
		value, err := env.Get(ast.(Symbol))
		if err != nil {
			return nil, lisperror.NewLispError(err, ast)
		}
		return value, nil
	} else if Q[List](ast) {
		lst := []MalType{}
		origList := ast.(List)
		for _, a := range origList.Val {
			exp, e := EVAL(ctx, a, env)
			if e != nil {
				// Preserve error and add context about which element failed
				return nil, e
			}
			lst = append(lst, exp)
		}
		return List{Val: lst, Cursor: origList.Cursor}, nil
	} else if Q[Vector](ast) {
		lst := []MalType{}
		origVec := ast.(Vector)
		for _, a := range origVec.Val {
			exp, e := EVAL(ctx, a, env)
			if e != nil {
				// Preserve error and add context about which element failed
				return nil, e
			}
			lst = append(lst, exp)
		}
		return Vector{Val: lst, Cursor: origVec.Cursor}, nil
	} else if Q[HashMap](ast) {
		m := ast.(HashMap)
		new_hm := HashMap{Val: map[string]MalType{}, Cursor: m.Cursor}
		for k, v := range m.Val {
			kv, e2 := EVAL(ctx, v, env)
			if e2 != nil {
				// Preserve error and add context about which key failed
				return nil, e2
			}
			new_hm.Val[k] = kv
		}
		return new_hm, nil
	} else {
		return ast, nil
	}
}

// extractFunctionName extracts the function, macro, or special form name from the AST
// Returns the name with "macro:" prefix if isMacro is true, or empty string if not applicable
func extractFunctionName(ast MalType, isMacro bool) string {
	if !Q[List](ast) {
		return "" // Not a function call
	}

	lst := ast.(List)
	if len(lst.Val) == 0 {
		return "" // Empty list
	}

	switch a0 := lst.Val[0].(type) {
	case Symbol:
		if isMacro {
			return "macro:" + a0.Val
		}
		return a0.Val
	case List:
		// Anonymous function: ((fn [...] ...) args)
		return "<lambda>"
	default:
		return "<unknown>"
	}
}

func do(ctx context.Context, ast MalType, from, to int, env EnvType) (MalType, error) {
	if ast == nil {
		return nil, nil
	}
	lst := ast.(List).Val
	if len(lst) == from {
		return nil, nil
	}
	evaledAST, e := eval_ast(ctx, List{Val: lst[from : len(lst)+to]}, env)
	if e != nil {
		return nil, e
	}
	evaledLst := evaledAST.(List).Val
	if to == 0 {
		return evaledLst[len(evaledLst)-1], nil
	}
	return lst[len(lst)-1], nil
}

// EVAL evaluates an Abstract Syntaxt Tree (AST) and returns a result (a reduced AST).
// It requires a context that might cancel execution, and requires an environment that might
// be modified.
// AST usually is generated by [READ] or [READWithPreamble].
func EVAL(ctx context.Context, ast MalType, env EnvType) (res MalType, e error) {
	// Extract function name before any processing (to capture macro names before expansion)
	isMacro := is_macro_call(ast, env)
	functionName := extractFunctionName(ast, isMacro)

	// Add stack frame to any error that propagates out of this EVAL call
	defer func() {
		if e != nil {
			if lispErr, ok := e.(lisperror.LispError); ok {
				e = lispErr.AddStackFrame(lisperror.GetPosition(ast), functionName)
			}
		}
	}()

	for {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return nil, lisperror.NewLispError(errors.New("timeout while evaluating expression"), ast)
			default:
			}
		}

		// DEBUG-EVAL support: print AST if DEBUG-EVAL is set and truthy
		if DebugEvalEnabled {
			if dbgEval, err := env.Get(Symbol{Val: "DEBUG-EVAL"}); err == nil {
				// Print if DEBUG-EVAL exists and is not nil or false
				switch dbgEval := dbgEval.(type) {
				case bool:
					if dbgEval {
						pos := lisperror.GetPosition(ast)
						if pos != nil {
							fmt.Printf("\033[38;5;208m%s\033[0m: %s\n", pos, PRINT(ast))
						}
					}
				default:
					// do nothing
				}
			}
		}

		switch ast := ast.(type) {
		case List: // continue
			// aStr, _ := PRINT(ast)
			// fmt.Printf("%s◉ %s\n", ast.Cursor, aStr)
		default:
			// aStr, _ := PRINT(ast)
			// fmt.Printf("%T○ %s\n", ast, aStr)
			return eval_ast(ctx, ast, env)
		}

		// apply list
		ast, e = macroexpand(ctx, ast, env)
		if e != nil {
			return nil, e
		}
		if !Q[List](ast) {
			return eval_ast(ctx, ast, env)
		}
		if len(ast.(List).Val) == 0 {
			return ast, nil
		}

		a0 := ast.(List).Val[0]
		var a1 MalType
		var a2 MalType
		switch len(ast.(List).Val) {
		case 1:
			a1 = nil
			a2 = nil
		case 2:
			a1 = ast.(List).Val[1]
			a2 = nil
		case 3:
			a1 = ast.(List).Val[1]
			a2 = ast.(List).Val[2]
		default:
			a1 = ast.(List).Val[1]
			a2 = ast.(List).Val[2]
		}
		a0sym := "__<*fn>__"
		if Q[Symbol](a0) {
			a0sym = a0.(Symbol).Val
		}
		switch a0sym {
		case "def":
			res, e := EVAL(ctx, a2, env)
			if e != nil {
				return nil, e
			}
			switch a1 := a1.(type) {
			case Symbol:
				return env.Set(a1, res), nil
			default:
				return nil, lisperror.NewLispError(fmt.Errorf("cannot use '%T' as identifier", a1), ast)
			}
		case "let":
			let_env := NewSubordinateEnv(env)
			arr1, e := GetSlice(a1)
			if e != nil {
				return nil, e
			}
			if len(arr1)%2 != 0 {
				return nil, lisperror.NewLispError(errors.New("let: odd elements on binding vector"), a1)
			}
			for i := 0; i < len(arr1); i += 2 {
				if !Q[Symbol](arr1[i]) {
					return nil, lisperror.NewLispError(errors.New("non-symbol bind value"), a1)
				}
				exp, e := EVAL(ctx, arr1[i+1], let_env)
				if e != nil {
					return nil, e
				}
				let_env.Set(arr1[i].(Symbol), exp)
			}
			astRef := ast.(List)
			ast, e = do(ctx, astRef, 2, -1, let_env)
			if e != nil {
				return nil, e
			}
			env = let_env
		case "quote": // '
			return a1, nil
		case "quasiquoteexpand":
			return quasiquote(a1), nil
		case "quasiquote": // `
			ast = quasiquote(a1)
		case "defmacro":
			fn, e := EVAL(ctx, a2, env)
			if e != nil {
				return nil, e
			}
			switch fn := fn.(type) {
			case MalFunc:
				return env.Set(a1.(Symbol), fn.SetMacro()), nil
			default:
				return nil, lisperror.NewLispError(fmt.Errorf("defmacro: second argument must be a function (was of type %T)", fn), ast)
			}
		case "macroexpand":
			return macroexpand(ctx, a1, env)
		case "try":
			lst := ast.(List).Val
			var last MalType
			var prelast MalType
			switch len(lst) {
			case 1:
				return nil, nil
			case 2:
				last = lst[1]
				prelast = nil
			case 3:
				last = lst[2]
				prelast = lst[1]
			default:
				last = lst[len(lst)-1]
				prelast = lst[len(lst)-2]
			}
			var tryDo, catchDo, finallyDo MalType // Lists
			var catchBind MalType                 // Symbol

			switch first(last) {
			case "catch":
				finallyDo = nil
				catchBind = last.(List).Val[1]
				catchDo = List{Val: last.(List).Val[2:]}
				tryDo = List{Val: lst[1 : len(lst)-1]}
				if len(catchDo.(List).Val) == 0 {
					return nil, lisperror.NewLispError(errors.New("catch must have 2 arguments at least"), ast)
				}
			case "finally":
				finallyDo = List{Val: last.(List).Val[1:]}
				switch first(prelast) {
				case "catch":
					catchBind = prelast.(List).Val[1]
					catchDo = List{Val: prelast.(List).Val[2:]}
					tryDo = List{Val: lst[1 : len(lst)-2]}
				default:
					catchBind = nil
					catchDo = nil
					tryDo = List{Val: lst[1 : len(lst)-1]}
				}
			default:
				finallyDo = nil
				catchBind = nil
				catchDo = nil
				tryDo = List{Val: lst[1:]}
			}
			exp, e := func() (res MalType, err error) {
				defer malRecover(&err)
				if dl, ok := ctx.Deadline(); ok {
					// give 80% of the time to the try, and the remaining 20% to the catch + finally
					timeout := (time.Until(dl) / 10) * 8
					ctx, cancel := context.WithTimeout(ctx, timeout)
					defer cancel()
					return do(ctx, tryDo, 0, 0, env)
				}
				return do(ctx, tryDo, 0, 0, env)
			}()

			defer func() { _, _ = do(ctx, finallyDo, 0, 0, env) }()

			if e == nil {
				return exp, nil
			} else {
				if catchDo != nil {
					var caughtError MalType
					if er, ok := e.(interface{ ErrorValue() MalType }); ok {
						caughtError = er.ErrorValue()
					} else {
						caughtError = e.Error()
					}
					binds := NewList(nil, catchBind)
					new_env, err := NewSubordinateEnvWithBinds(env, binds, NewList(nil, caughtError))
					if err != nil {
						return nil, err
					}
					ast, err = do(ctx, catchDo, 0, 0, new_env)
					if err != nil {
						return nil, err
					}
					env = new_env
					continue
				}
				return nil, e
			}
		case "do":
			var err error
			ast, err = do(ctx, ast, 1, -1, env)
			if err != nil {
				return nil, err
			}
		case "if":
			cond, e := EVAL(ctx, a1, env)
			if e != nil {
				return nil, e
			}
			if cond == nil || cond == false {
				if len(ast.(List).Val) >= 4 {
					ast = ast.(List).Val[3]
				} else {
					return nil, nil
				}
			} else {
				ast = a2
			}
		case "fn":
			fn := MalFunc{
				Eval:    EVAL,
				Exp:     List{Val: append([]MalType{Symbol{Val: "do"}}, ast.(List).Val[2:]...)},
				Env:     env,
				Params:  a1,
				IsMacro: false,
				GenEnv:  NewSubordinateEnvWithBinds,
				Meta:    nil,
				Cursor:  ast.(List).Cursor,
			}
			return fn, nil
		default:
			el, e := eval_ast(ctx, ast, env)
			if e != nil {
				return nil, e
			}
			f := el.(List).Val[0]
			if Q[MalFunc](f) {
				fn := f.(MalFunc)
				ast = fn.Exp
				env, e = NewSubordinateEnvWithBinds(fn.Env, fn.Params, List{Val: el.(List).Val[1:]})
				if e != nil {
					if ast == nil {
						return nil, lisperror.NewLispError(e, nil)
					}
					switch v := ast.(List).Val[0].(type) {
					case Symbol:
						return nil, lisperror.NewLispError(fmt.Errorf("%s (around %s)", e, v.Val), ast)
					default:
						return nil, lisperror.NewLispError(e, ast)
					}
				}
			} else {
				fn, ok := f.(Func)
				if !ok {
					return nil, lisperror.NewLispError(fmt.Errorf("attempt to call non-function (was of type %T)", f), el)
				}
				result, err := fn.Fn(ctx, el.(List).Val[1:])
				if err != nil {
					return nil, lisperror.NewLispError(err, ast)
				}
				return result, nil
			}
		}
	} // TCO loop
}

func first(list MalType) string {
	if list != nil && Q[List](list) && Q[Symbol](list.(List).Val[0]) {
		return list.(List).Val[0].(Symbol).Val
	}
	return ""
}

func malRecover(err *error) {
	rerr := recover()
	if rerr != nil {
		*err = rerr.(error)
	}
}

// PRINT converts an AST to a string, suitable for printing
// AST might be generated by [EVAL] or by [READ] or [READWithPreamble].
func PRINT(ast MalType) string {
	return printer.Pr_str(ast, true)
}

// REPL or [READ], [EVAL] and [PRINT] loop execute those three functions in sequence.
// (but the loop "L" actually must be executed by the caller)
func REPL(ctx context.Context, env EnvType, sourceCode string, cursor *Position) (MalType, error) {
	ast, err := READ(sourceCode, cursor, env)
	if err != nil {
		return nil, err
	}
	exp, err := EVAL(ctx, ast, env)
	if err != nil {
		return nil, err
	}
	return PRINT(exp), nil
}

// REPLWithPreamble or [READ], [EVAL] and [PRINT] loop with preamble execute those three functions in sequence.
// (but the loop "L" actually must be executed by the caller)
//
// Source code might include a preamble with the values for the placeholders. See [READWithPreamble]
func REPLWithPreamble(ctx context.Context, env EnvType, sourceCode string, cursor *Position) (MalType, error) {
	ast, err := READWithPreamble(sourceCode, cursor, env)
	if err != nil {
		return nil, err
	}
	exp, err := EVAL(ctx, ast, env)
	if err != nil {
		return nil, err
	}
	return PRINT(exp), nil
}

// ReadEvalWithPreamble or [READ] and [EVAL] with preamble execute those three functions in sequence.
// (but the loop "L" actually must be executed by the caller)
//
// Source code might include a preamble with the values for the placeholders. See [READWithPreamble]
// ReadEvalWithPreamble returns the result in AST structure.
func ReadEvalWithPreamble(ctx context.Context, env EnvType, sourceCode string, cursor *Position) (MalType, error) {
	ast, err := READWithPreamble(sourceCode, cursor, env)
	if err != nil {
		return nil, err
	}
	return EVAL(ctx, ast, env)
}
