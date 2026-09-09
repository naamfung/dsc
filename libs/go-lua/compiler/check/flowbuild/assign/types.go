package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// PredicateLinkFunc extracts a predicate link from a call site.
type PredicateLinkFunc func(
	callInfo *cfg.CallInfo,
	p cfg.Point,
	sc *scope.State,
	inputs *flow.Inputs,
	synth func(ast.Expr, cfg.Point) typ.Type,
	constResolver func(string) *flow.ConstValue,
) *flow.PredicateLink

// KeysCollectorFunc detects if a call is to a "keys collector" function.
type KeysCollectorFunc func(callInfo *cfg.CallInfo, p cfg.Point, retIndex int) cfg.SymbolID
