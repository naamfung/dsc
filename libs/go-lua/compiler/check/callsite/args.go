package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// PositionalArgAt returns the positional argument at idx.
// Negative indices address from the end (-1 is last).
func PositionalArgAt(args []ast.Expr, idx int) ast.Expr {
	if len(args) == 0 {
		return nil
	}
	if idx < 0 {
		idx = len(args) + idx
	}
	if idx < 0 || idx >= len(args) {
		return nil
	}
	return args[idx]
}

// RuntimeArgCount returns call runtime arity, including receiver for method calls.
func RuntimeArgCount(info *cfg.CallInfo) int {
	if info == nil {
		return 0
	}
	if IsMethodCallInfo(info) {
		return len(info.Args) + 1
	}
	return len(info.Args)
}

// RuntimeArgAt returns the runtime argument at parameter index paramIdx.
//
// For method calls, index 0 is the receiver (self), and remaining indices map
// to positional call args. Negative indices address from the runtime argument
// tail, including the receiver slot.
func RuntimeArgAt(info *cfg.CallInfo, paramIdx int) ast.Expr {
	if info == nil {
		return nil
	}
	if IsMethodCallInfo(info) {
		if paramIdx == 0 {
			return info.Receiver
		}
		if paramIdx < 0 {
			adj := RuntimeArgCount(info) + paramIdx
			if adj == 0 {
				return info.Receiver
			}
			return PositionalArgAt(info.Args, adj-1)
		}
		return PositionalArgAt(info.Args, paramIdx-1)
	}
	return PositionalArgAt(info.Args, paramIdx)
}
