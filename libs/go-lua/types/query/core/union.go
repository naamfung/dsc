package core

import (
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/union"
)

// CompatibleFunctionFromUnion finds a compatible function signature from union members.
//
// When a union type contains multiple function signatures, this selects or merges
// them based on the expected parameter count. The selection strategy:
//
//  1. If only one function exists, return it
//  2. Keep only functions whose arity can accept paramCount
//     (respecting required params, optional params, and variadics)
//  3. If multiple candidates remain, merge them into a combined signature
//
// This is used during function call type checking when the callee has a union
// type containing multiple function overloads.
//
// Returns nil if no function types exist in the union or no function is arity-compatible.
func CompatibleFunctionFromUnion(paramCount int, expected typ.Type) *typ.Function {
	fns := union.FunctionTypes(expected)
	if len(fns) == 0 {
		return nil
	}

	if len(fns) == 1 {
		if acceptsArity(fns[0], paramCount) {
			return fns[0]
		}
		return nil
	}

	var matching []*typ.Function
	for _, f := range fns {
		if acceptsArity(f, paramCount) {
			matching = append(matching, f)
		}
	}

	if len(matching) == 0 {
		return nil
	}

	if len(matching) == 1 {
		return matching[0]
	}

	return union.MergeFunctions(matching)
}

func acceptsArity(fn *typ.Function, argCount int) bool {
	if fn == nil {
		return false
	}
	if argCount < typ.MinRequiredArgs(fn) {
		return false
	}
	if fn.Variadic != nil {
		return true
	}
	return argCount <= len(fn.Params)
}
