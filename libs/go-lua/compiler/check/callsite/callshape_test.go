package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestMethodCallInfoHelpers(t *testing.T) {
	if IsMethodCallInfo(nil) || IsMethodLikeCallInfo(nil) {
		t.Fatal("nil CallInfo must not be method-shaped")
	}

	partialMethodOnly := &cfg.CallInfo{Method: "run"}
	if IsMethodCallInfo(partialMethodOnly) {
		t.Fatal("method-only CallInfo must not be fully-formed method call")
	}
	if !IsMethodLikeCallInfo(partialMethodOnly) {
		t.Fatal("method-only CallInfo should be method-like")
	}

	partialReceiverOnly := &cfg.CallInfo{Receiver: &ast.IdentExpr{Value: "self"}}
	if IsMethodCallInfo(partialReceiverOnly) {
		t.Fatal("receiver-only CallInfo must not be fully-formed method call")
	}
	if !IsMethodLikeCallInfo(partialReceiverOnly) {
		t.Fatal("receiver-only CallInfo should be method-like")
	}

	full := &cfg.CallInfo{Method: "run", Receiver: &ast.IdentExpr{Value: "self"}}
	if !IsMethodCallInfo(full) || !IsMethodLikeCallInfo(full) {
		t.Fatal("fully-formed method CallInfo should satisfy both helpers")
	}
}

func TestMethodExprHelpers(t *testing.T) {
	if IsMethodLikeExpr(nil) {
		t.Fatal("nil call expr must not be method-shaped")
	}

	partialMethodOnly := &ast.FuncCallExpr{Method: "run"}
	if !IsMethodLikeExpr(partialMethodOnly) {
		t.Fatal("method-only expression should be method-like")
	}

	partialReceiverOnly := &ast.FuncCallExpr{Receiver: &ast.IdentExpr{Value: "self"}}
	if !IsMethodLikeExpr(partialReceiverOnly) {
		t.Fatal("receiver-only expression should be method-like")
	}

	full := &ast.FuncCallExpr{Method: "run", Receiver: &ast.IdentExpr{Value: "self"}}
	if !IsMethodLikeExpr(full) {
		t.Fatal("fully-formed method expression should be method-like")
	}
}
