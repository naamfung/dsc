package infer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/typ"
)

// iterateSCCFixpoint runs fixpoint iteration for a single SCC until convergence.
// Returns true if types stabilized within the iteration limit.
func (i *Inferencer) iterateSCCFixpoint(
	run RunContext,
	scc []cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) bool {
	for iter := 0; iter < i.maxIterations; iter++ {
		next, changed := i.runSCCIteration(run, scc, localFuncs, summaries)
		applySCCIterationUpdates(summaries, scc, next)
		if !changed {
			return true
		}
	}
	return false
}

func (i *Inferencer) planLocalFunctionSCCs(localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo) [][]cfg.SymbolID {
	// Propagate inter-procedural parameter hints across local call edges before
	// SCC return inference so unannotated params get stable callsite-driven seeds.
	returns.PropagateParamHintsFromCallGraph(localFuncs)

	var moduleBindings *bind.BindingTable
	if i != nil && i.store != nil {
		moduleBindings = i.store.ModuleBindings()
	}
	adj := returns.BuildLocalCallGraph(localFuncs, moduleBindings)
	return returns.ComputeSymbolSCCs(adj)
}

func seedSummariesFromSeed(
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	seed map[cfg.SymbolID][]typ.Type,
) map[cfg.SymbolID][]typ.Type {
	summaries := make(map[cfg.SymbolID][]typ.Type, len(localFuncs))
	if seed == nil {
		return summaries
	}
	for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
		if seeded := seed[sym]; len(seeded) > 0 {
			summaries[sym] = seeded
		}
	}
	return summaries
}

func (i *Inferencer) processSCCSummaries(
	run RunContext,
	sccs [][]cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) []diag.Diagnostic {
	var diags []diag.Diagnostic
	for _, scc := range sccs {
		if len(scc) == 0 {
			continue
		}
		if i.iterateSCCFixpoint(run, scc, localFuncs, summaries) {
			continue
		}
		if warn := i.widenSCCToUnknown(scc, localFuncs, summaries); warn != nil {
			diags = append(diags, *warn)
		}
	}
	return diags
}

func (i *Inferencer) runSCCIteration(
	run RunContext,
	scc []cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) (map[cfg.SymbolID][]typ.Type, bool) {
	changed := false
	next := make(map[cfg.SymbolID][]typ.Type, len(scc))
	for _, sym := range scc {
		info := localFuncs[sym]
		if info == nil || info.Fn == nil {
			continue
		}
		newReturn := i.inferReturnWithSummary(run, info, summaries, localFuncs)
		oldReturn := summaries[sym]
		merged := returns.MergeReturnSummary(oldReturn, newReturn)
		next[sym] = merged
		if !returns.ReturnTypesEqual(merged, oldReturn) {
			changed = true
		}
	}
	return next, changed
}

func applySCCIterationUpdates(
	summaries map[cfg.SymbolID][]typ.Type,
	scc []cfg.SymbolID,
	next map[cfg.SymbolID][]typ.Type,
) {
	for _, sym := range scc {
		if v, ok := next[sym]; ok {
			summaries[sym] = v
		}
	}
}

// widenSCCToUnknown widens all SCC members to unknown when fixpoint did not converge.
// Preserves return arity while replacing type slots with unknown.
func (i *Inferencer) widenSCCToUnknown(
	scc []cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) *diag.Diagnostic {
	for _, sym := range scc {
		existing := summaries[sym]
		if len(existing) == 0 {
			summaries[sym] = []typ.Type{typ.Unknown}
		} else {
			widened := make([]typ.Type, len(existing))
			for i := range widened {
				widened[i] = typ.Unknown
			}
			summaries[sym] = widened
		}
	}
	if info := localFuncs[scc[0]]; info != nil && info.Fn != nil {
		return &diag.Diagnostic{
			Position: diag.Position{File: i.sourceName, Line: info.Fn.Line(), Column: info.Fn.Column()},
			Span:     ast.SpanOf(info.Fn),
			Severity: diag.SeverityWarning,
			Message:  "return type fixpoint did not converge; using unknown",
		}
	}
	return nil
}
