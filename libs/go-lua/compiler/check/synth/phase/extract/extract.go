// Package extract provides type synthesis for pre-flow phases (A and B).
// It does NOT access flow narrowing or apply return transforms.
package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// Synth provides expression type synthesis for pre-flow phases.
// Extends api.Synth with extract-specific spec overlay support.
type Synth interface {
	api.Synth

	ExpandValuesWithSpecTypes(exprs []ast.Expr, needed int, p cfg.Point, specTypes api.SpecTypes) []typ.Type
	InferIterVarsWithSpecTypes(exprs []ast.Expr, count int, p cfg.Point, specTypes api.SpecTypes) []typ.Type
	SynthExprAt(expr ast.Expr, p cfg.Point, sc *scope.State) typ.Type
}
