package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// IsMethodCallInfo reports true only for fully-formed method callsite info.
func IsMethodCallInfo(info *cfg.CallInfo) bool {
	return info != nil && info.Method != "" && info.Receiver != nil
}

// IsMethodLikeCallInfo reports true when callsite info carries any method shape.
func IsMethodLikeCallInfo(info *cfg.CallInfo) bool {
	return info != nil && (info.Method != "" || info.Receiver != nil)
}

// IsMethodLikeExpr reports true when an expression carries any method shape.
func IsMethodLikeExpr(ex *ast.FuncCallExpr) bool {
	return ex != nil && (ex.Method != "" || ex.Receiver != nil)
}
