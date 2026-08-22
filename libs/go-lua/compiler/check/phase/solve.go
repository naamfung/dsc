package phase

import (
	"github.com/wippyai/go-lua/types/flow"
)

// RunSolve executes the flow solve phase.
func RunSolve(input FlowSolveInput) FlowSolveOutput {
	var solution *flow.Solution
	if input.Extract.Inputs != nil {
		solution = flow.Solve(input.Extract.Inputs, input.Resolver)
	}
	return FlowSolveOutput{
		Solution: solution,
	}
}
