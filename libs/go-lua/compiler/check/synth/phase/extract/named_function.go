package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	compcfg "github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// functionLiteralForIdent resolves an identifier to its underlying function
// literal when the symbol is bound to a local function definition/literal.
func (s *Synthesizer) functionLiteralForIdent(ident *ast.IdentExpr) *ast.FunctionExpr {
	if ident == nil {
		return nil
	}

	var graph *compcfg.Graph
	if s.deps.CheckCtx != nil {
		if g, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph); ok {
			graph = g
		}
	}

	bindings := s.deps.ModuleBindings
	if graph != nil && graph.Bindings() != nil {
		bindings = graph.Bindings()
	}
	moduleBindings := s.deps.ModuleBindings

	hasFunctionLiteral := func(sym compcfg.SymbolID) bool {
		if sym == 0 {
			return false
		}
		if fn := callsite.FunctionLiteralForSymbol(graph, bindings, sym); fn != nil {
			return true
		}
		if moduleBindings != nil && moduleBindings != bindings {
			return callsite.FunctionLiteralForSymbol(graph, moduleBindings, sym) != nil
		}
		return false
	}

	sym := callsite.CanonicalSymbolFromExprWithAliases(ident, 0, graph, bindings, moduleBindings, hasFunctionLiteral)
	if sym == 0 {
		return nil
	}
	if fn := callsite.FunctionLiteralForSymbol(graph, bindings, sym); fn != nil {
		return fn
	}
	if moduleBindings != nil && moduleBindings != bindings {
		if fn := callsite.FunctionLiteralForSymbol(graph, moduleBindings, sym); fn != nil {
			return fn
		}
	}

	return nil
}

// graphLocalFunctionLiteralForExpr resolves an expression to a graph-local stable
// function literal when one exists.
//
// Canonical boundary:
//   - include alias-expanded graph-local function definitions and local identifier
//     assignments of function literals
//   - exclude mutable field-path symbols, which must continue to read their
//     current callable type from value flow
func (s *Synthesizer) graphLocalFunctionForExpr(expr ast.Expr) (compcfg.SymbolID, *ast.FunctionExpr, bool) {
	if expr == nil || s == nil || s.deps.CheckCtx == nil {
		return 0, nil, false
	}

	graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return 0, nil, false
	}

	bindings := graph.Bindings()
	if bindings == nil {
		bindings = s.deps.ModuleBindings
	}
	moduleBindings := s.deps.ModuleBindings

	hasGraphLocalLiteral := func(sym compcfg.SymbolID) bool {
		return callsite.FunctionLiteralForGraphSymbol(graph, sym) != nil
	}

	raw := callsite.SymbolFromExpr(expr, bindings)
	if raw == 0 && moduleBindings != nil && moduleBindings != bindings {
		raw = callsite.SymbolFromExpr(expr, moduleBindings)
	}

	sym := callsite.CanonicalSymbolFromExprWithAliases(
		expr,
		raw,
		graph,
		bindings,
		moduleBindings,
		hasGraphLocalLiteral,
	)
	if sym == 0 {
		return 0, nil, false
	}

	fn := callsite.FunctionLiteralForGraphSymbol(graph, sym)
	if fn == nil {
		return 0, nil, false
	}

	captureBindings := bindings
	if captureBindings == nil {
		captureBindings = moduleBindings
	}
	hasCaptures := captureBindings != nil && len(captureBindings.CapturedSymbols(fn)) > 0

	return sym, fn, hasCaptures
}

func (s *Synthesizer) graphLocalFunctionLiteralForExpr(expr ast.Expr) *ast.FunctionExpr {
	_, fn, _ := s.graphLocalFunctionForExpr(expr)
	return fn
}

func (s *Synthesizer) hasDominatingDirectFunctionRebind(sym compcfg.SymbolID, stableFn *ast.FunctionExpr, p cfg.Point) bool {
	if s == nil || sym == 0 || stableFn == nil || s.deps == nil || s.deps.CheckCtx == nil {
		return false
	}

	graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return false
	}

	idom, _ := cfganalysis.ComputeDominators(graph.CFG())
	rebound := false

	graph.EachAssign(func(assignPoint cfg.Point, info *compcfg.AssignInfo) {
		if rebound || info == nil || assignPoint == p || !cfganalysis.StrictlyDominates(idom, assignPoint, p) {
			return
		}

		info.EachTarget(func(_ int, target compcfg.AssignTarget) {
			if rebound || target.Symbol != sym {
				return
			}
			if target.Kind == compcfg.TargetField || target.Kind == compcfg.TargetIndex {
				rebound = true
			}
		})
	})

	if rebound {
		return true
	}

	graph.EachFuncDef(func(defPoint cfg.Point, info *compcfg.FuncDefInfo) {
		if rebound || info == nil || info.Symbol != sym || info.FuncExpr == nil || info.FuncExpr == stableFn {
			return
		}
		if !cfganalysis.StrictlyDominates(idom, defPoint, p) {
			return
		}
		if info.TargetKind == compcfg.FuncDefField || info.TargetKind == compcfg.FuncDefGlobal {
			rebound = true
		}
	})

	return rebound
}

func (s *Synthesizer) expectedGraphLocalFunctionValueType(
	expr ast.Expr,
	p cfg.Point,
	sc *scope.State,
	expected *typ.Function,
	captureTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	if s == nil || expected == nil {
		return nil
	}

	sym, fn, _ := s.graphLocalFunctionForExpr(expr)
	if fn == nil {
		return nil
	}
	if s.hasDominatingDirectFunctionRebind(sym, fn, p) {
		return nil
	}

	return s.synthFunctionTypeWithCapturePoint(fn, sc, expected, p, captureTypes)
}

func (s *Synthesizer) stableGraphLocalFunctionSnapshotType(sym compcfg.SymbolID) typ.Type {
	if s == nil || sym == 0 || s.deps == nil || s.deps.Ctx == nil || s.deps.CheckCtx == nil {
		return nil
	}

	store := api.StoreFrom(s.deps.Ctx)
	if store == nil {
		return nil
	}

	graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return nil
	}

	fallbackParent := s.deps.DefaultScope
	if fallbackParent == nil {
		fallbackParent = s.deps.CheckCtx.TypeNames()
	}
	parent := api.ParentScopeForGraph(store, graph.ID(), fallbackParent)
	if parent == nil {
		return nil
	}

	var fnTypes map[cfg.SymbolID]typ.Type
	load := func() {
		fnTypes = store.GetLocalFuncTypesSnapshot(graph, parent)
	}
	if phaser, ok := store.(interface{ WithPhase(api.Phase, func()) }); ok {
		phaser.WithPhase(api.PhaseScopeCompute, load)
	} else {
		load()
	}
	if len(fnTypes) == 0 {
		return nil
	}

	return fnTypes[sym]
}

func (s *Synthesizer) stableLocalFunctionValueType(
	expr ast.Expr,
	p cfg.Point,
	sc *scope.State,
	current typ.Type,
	captureTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	sym, fn, hasCaptures := s.graphLocalFunctionForExpr(expr)
	if fn == nil {
		return nil
	}

	authoritative := current
	if s.deps != nil && s.deps.CheckCtx != nil {
		if types := s.deps.CheckCtx.Types(); types != nil {
			if tv := types.EffectiveTypeAt(p, sym); tv.State == flow.StateResolved && tv.Type != nil {
				authoritative = tv.Type
			}
		}
	}
	if snapshot := s.stableGraphLocalFunctionSnapshotType(sym); snapshot != nil {
		if authoritative == nil || subtype.IsSubtype(snapshot, authoritative) {
			authoritative = snapshot
		}
	}
	if !hasCaptures && authoritative != nil {
		return authoritative
	}

	expectedFn, _ := unwrap.Optional(unwrap.Alias(authoritative)).(*typ.Function)
	specialized := s.synthFunctionTypeWithCapturePoint(fn, sc, expectedFn, p, captureTypes)
	if authoritative != nil && specialized != nil {
		if subtype.IsSubtype(specialized, authoritative) {
			return specialized
		}
		return authoritative
	}
	if authoritative != nil {
		return authoritative
	}
	return specialized
}
