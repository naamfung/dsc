package lnotation

import (
	. "github.com/jig/lisp/types"
)

// S converts argument string to a lisp symbol
func S(arg string) Symbol {
	return Symbol{Val: arg}
}

// L returns a lisp list of its arguments
func L(args ...MalType) List {
	return List{Val: args}
}

// LS is a helper to call (f ...) lisp forms
func LS(symbol string, args ...MalType) List {
	return List{
		Val: append(
			[]MalType{
				Symbol{Val: symbol},
			},
			args...,
		),
	}
}

// V returns a lisp vector of its arguments
func V[T any](args []T) Vector {
	result := []MalType{}
	for _, k := range args {
		result = append(result, k)
	}
	return Vector{Val: result}
}

// M converts Go map to lisp HashMap
func HM(arg map[string]interface{}) HashMap {
	result := map[string]MalType{}
	for k, v := range arg {
		switch v := v.(type) {
		case map[string]interface{}:
			result[k] = HM(v)
		default:
			result[k] = v
		}
	}
	return HashMap{Val: result}
}

func SET(args []string) Set {
	result := map[string]struct{}{}
	for _, k := range args {
		result[k] = struct{}{}
	}
	return Set{Val: result}
}
