package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ReconcileFunctionFactInput captures all channels that can influence a single
// local-function fact slot during one update step.
type ReconcileFunctionFactInput struct {
	ExistingSummary []typ.Type
	ExistingNarrow  []typ.Type
	ExistingFunc    typ.Type

	CandidateSummary []typ.Type
	CandidateNarrow  []typ.Type
	CandidateFunc    typ.Type
}

// ReconcileFunctionFactOutput is the canonical reconciled state for one symbol.
type ReconcileFunctionFactOutput struct {
	Summary []typ.Type
	Narrow  []typ.Type
	Func    typ.Type
}

// FunctionFactCandidate captures incoming candidate data for one symbol's
// function-related fact channels.
type FunctionFactCandidate struct {
	Summary []typ.Type
	Narrow  []typ.Type
	Func    typ.Type
}

// ReconcileFunctionFact centralizes reconciliation of return summary, narrow
// return summary, and function type for one symbol.
//
// This is the only policy entrypoint for function-fact channel convergence.
func ReconcileFunctionFact(in ReconcileFunctionFactInput) ReconcileFunctionFactOutput {
	out := ReconcileFunctionFactOutput{
		Summary: NormalizeReturnVector(in.ExistingSummary),
		Narrow:  NormalizeReturnVector(in.ExistingNarrow),
		Func:    in.ExistingFunc,
	}

	if len(in.CandidateSummary) > 0 {
		out.Summary = MergeReturnSummary(out.Summary, in.CandidateSummary)
	}
	if len(in.CandidateNarrow) > 0 {
		out.Narrow = MergeReturnSummary(out.Narrow, in.CandidateNarrow)
	}
	if in.CandidateFunc != nil {
		out.Func = MergeFunctionFactType(out.Func, in.CandidateFunc)
	}

	// Keep summary and narrow channels mutually refining when post-flow narrow
	// provides first-order information. MergeReturnSummary is the canonical
	// policy and already encodes directional refinement preference.
	if len(out.Narrow) > 0 {
		if len(out.Summary) == 0 {
			out.Summary = NormalizeReturnVector(out.Narrow)
		} else {
			out.Summary = MergeReturnSummary(out.Summary, out.Narrow)
		}
	}

	if fn := unwrap.Function(out.Func); fn != nil {
		alignedSummary := out.Summary
		if len(out.Narrow) > 0 {
			// Canonical tie-breaker: function facts track post-flow behavior.
			// Narrow summaries are produced from solved flow and are authoritative
			// for call-site typing in the current iteration.
			alignedSummary = out.Narrow
		}
		if len(alignedSummary) > 0 {
			if aligned, changed := AlignFunctionTypeWithSummary(fn, alignedSummary); changed {
				out.Func = aligned
				fn = aligned
			}
		}
		if len(out.Summary) == 0 && fn != nil && len(fn.Returns) > 0 {
			out.Summary = NormalizeReturnVector(fn.Returns)
		}
	}

	return out
}

// MergeFunctionFactIntoFacts reconciles and writes function-related facts for
// one symbol into a facts bundle using canonical kernel policy.
func MergeFunctionFactIntoFacts(facts *api.Facts, sym cfg.SymbolID, candidate FunctionFactCandidate) {
	if facts == nil || sym == 0 {
		return
	}
	NormalizeFunctionFactChannels(facts)
	mergeFunctionFactIntoNormalizedFacts(facts, sym, candidate)
}

func mergeFunctionFactIntoNormalizedFacts(facts *api.Facts, sym cfg.SymbolID, candidate FunctionFactCandidate) {
	existing := readFunctionFactFromFacts(facts, sym)
	reconciled := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary:  existing.Summary,
		ExistingNarrow:   existing.Narrow,
		ExistingFunc:     existing.Func,
		CandidateSummary: candidate.Summary,
		CandidateNarrow:  candidate.Narrow,
		CandidateFunc:    candidate.Func,
	})
	writeFunctionFactToFacts(facts, sym, api.FunctionFact{
		Summary: reconciled.Summary,
		Narrow:  reconciled.Narrow,
		Func:    reconciled.Func,
	})
}

// MergeFunctionFactsIntoFacts merges full function-fact channel maps into facts
// via the canonical single-symbol reconciliation path.
func MergeFunctionFactsIntoFacts(
	facts *api.Facts,
	summaries api.ReturnSummaries,
	narrows api.NarrowReturnSummaries,
	funcs api.FuncTypes,
) {
	if facts == nil {
		return
	}
	NormalizeFunctionFactChannels(facts)
	for _, sym := range collectFunctionFactChannelSymbols(summaries, narrows, funcs, nil) {
		mergeFunctionFactIntoNormalizedFacts(facts, sym, FunctionFactCandidate{
			Summary: summaries[sym],
			Narrow:  narrows[sym],
			Func:    funcs[sym],
		})
	}
}
