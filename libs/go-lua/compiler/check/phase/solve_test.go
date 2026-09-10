package phase

import (
	"testing"

	"github.com/wippyai/go-lua/types/flow"
)

func TestRunSolve_NilInputs(t *testing.T) {
	input := FlowSolveInput{
		Extract: FlowExtractOutput{Inputs: nil},
	}
	output := RunSolve(input)
	if output.Solution != nil {
		t.Error("expected nil solution for nil inputs")
	}
}

func TestRunSolve_ExplicitNilInputs(t *testing.T) {
	output := RunSolve(FlowSolveInput{
		Extract:  FlowExtractOutput{Inputs: nil},
		Resolver: nil,
	})
	if output.Solution != nil {
		t.Error("expected nil solution for nil inputs")
	}
}

func TestRunSolve_EmptyInputs(t *testing.T) {
	inputs := &flow.Inputs{}
	input := FlowSolveInput{
		Extract: FlowExtractOutput{Inputs: inputs},
	}
	output := RunSolve(input)
	if output.Solution == nil {
		t.Error("expected non-nil solution for non-nil inputs")
	}
}

func TestRunSolve_ExplicitResolverPath(t *testing.T) {
	inputs := &flow.Inputs{}
	output := RunSolve(FlowSolveInput{
		Extract:  FlowExtractOutput{Inputs: inputs},
		Resolver: nil,
	})
	if output.Solution == nil {
		t.Error("expected non-nil solution for non-nil inputs")
	}
}
