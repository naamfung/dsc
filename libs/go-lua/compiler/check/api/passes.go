package api

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
)

// ComputePass computes additional analysis artifacts during function analysis.
// Unlike Pass (which runs after convergence), ComputePass runs during each iteration
// as part of the phase pipeline. Results are memoized in FuncResult.Extras.
//
// Use ComputePass for analysis that needs to participate in the fixpoint loop
// or that computes data structures needed by later analysis phases. The Name()
// method provides the key under which results are stored in Extras.
type ComputePass interface {
	Name() string
	Run(graph *cfg.Graph, scopes map[cfg.Point]*scope.State) any
}
