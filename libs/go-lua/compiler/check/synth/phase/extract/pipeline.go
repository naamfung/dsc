package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ArgReSynth is called to re-synthesize an argument with contextual typing.
type ArgReSynth func(idx int, arg ast.Expr, expected typ.Type) typ.Type

// CallPipeline executes the two-phase call synthesis flow.
type CallPipeline struct {
	ctx      *db.QueryContext
	def      ops.CallDef
	astArgs  []ast.Expr
	reSynth  ArgReSynth
	infer    ops.InferResult
	finished bool
}

// NewCallPipeline creates a new call pipeline with the given definition.
func NewCallPipeline(ctx *db.QueryContext, def ops.CallDef, astArgs []ast.Expr) *CallPipeline {
	return &CallPipeline{
		ctx:     ctx,
		def:     def,
		astArgs: astArgs,
	}
}

// WithReSynth sets the re-synthesis callback for contextual typing.
func (p *CallPipeline) WithReSynth(reSynth ArgReSynth) *CallPipeline {
	p.reSynth = reSynth
	return p
}

// WithExpected sets the expected return type for bidirectional generic inference.
func (p *CallPipeline) WithExpected(expected typ.Type) *CallPipeline {
	p.def.ExpectedReturn = expected
	return p
}

// Infer runs Phase 1: callee resolution and type argument inference.
func (p *CallPipeline) Infer() ops.InferResult {
	p.infer = ops.InferCall(p.ctx, p.def)
	return p.infer
}

// ExpectedArgType returns the expected type for argument at index idx.
func (p *CallPipeline) ExpectedArgType(idx int) typ.Type {
	if idx < len(p.infer.ExpectedArgs) {
		return p.infer.ExpectedArgs[idx]
	}
	return p.infer.ExpectedVariadic
}

// ReSynthAndReInfer runs Phase 2: re-synthesizes arguments and re-infers if needed.
func (p *CallPipeline) ReSynthAndReInfer() bool {
	if p.reSynth == nil || len(p.astArgs) == 0 {
		return false
	}

	updatedArgs, changed := p.reSynthArgs()
	if !changed {
		return false
	}

	p.def.Args = updatedArgs
	if len(p.def.TypeArgs) == 0 {
		p.infer = ops.ReInfer(p.ctx, p.def, p.infer)
	}
	return true
}

// Finish runs Phase 3: completes the call and returns the result.
func (p *CallPipeline) Finish() ops.CallResult {
	p.finished = true
	return ops.FinishCall(p.ctx, p.def, p.infer)
}

// Run executes the full pipeline: Infer -> ReSynthAndReInfer -> Finish.
func (p *CallPipeline) Run() ops.CallResult {
	p.Infer()
	p.ReSynthAndReInfer()
	return p.Finish()
}

// reSynthArgs re-synthesizes arguments using the callback.
func (p *CallPipeline) reSynthArgs() ([]typ.Type, bool) {
	result := make([]typ.Type, len(p.astArgs))
	copy(result, p.def.Args)
	changed := false

	for i, arg := range p.astArgs {
		expected := p.ExpectedArgType(i)
		if expected == nil {
			continue
		}

		reSynthed := p.reSynth(i, arg, expected)
		if reSynthed != nil {
			result[i] = reSynthed
			changed = true
		}
	}

	return result, changed
}

// FunctionLiteralReSynth creates an ArgReSynth that only re-synthesizes function literals.
func FunctionLiteralReSynth(synthFn func(fn *ast.FunctionExpr, expected *typ.Function) typ.Type) ArgReSynth {
	return func(idx int, arg ast.Expr, expected typ.Type) typ.Type {
		fnExpr, ok := arg.(*ast.FunctionExpr)
		if !ok {
			return nil
		}
		expectedFn, ok := unwrap.Alias(expected).(*typ.Function)
		if !ok {
			return nil
		}
		return synthFn(fnExpr, expectedFn)
	}
}

// TableCompatChecker checks if a table literal is compatible with an expected type.
type TableCompatChecker func(table *ast.TableExpr, expected typ.Type, p cfg.Point) bool

// FullArgReSynth creates an ArgReSynth that re-synthesizes function and table literals.
func FullArgReSynth(
	synthWithExpected func(arg ast.Expr, p cfg.Point, expected typ.Type) typ.Type,
	tableChecker TableCompatChecker,
	p cfg.Point,
) ArgReSynth {
	return func(idx int, arg ast.Expr, expected typ.Type) typ.Type {
		switch a := arg.(type) {
		case *ast.FunctionExpr:
			return synthWithExpected(a, p, expected)
		case *ast.TableExpr:
			if tableChecker != nil && tableChecker(a, expected, p) {
				return expected
			}
			return synthWithExpected(a, p, expected)
		case *ast.IdentExpr:
			return synthWithExpected(a, p, expected)
		}
		return nil
	}
}
