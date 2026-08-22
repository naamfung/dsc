package pipeline

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewRunner(t *testing.T) {
	globals := map[string]typ.Type{"print": typ.Any}
	r := NewRunner(RunnerConfig{
		GlobalTypes:   globals,
		MaxScopeDepth: 10,
	})
	if r == nil {
		t.Fatal("expected non-nil runner")
	}
	if r.globalTypes["print"] != typ.Any {
		t.Error("globals not set")
	}
	if r.maxScopeDepth != 10 {
		t.Error("maxScopeDepth not set")
	}
}

func TestRunner_Run_NilStore(t *testing.T) {
	r := NewRunner(RunnerConfig{})
	result := r.Run(nil, api.FuncKey{})
	if result != nil {
		t.Error("expected nil when context is nil")
	}
}

func TestRunnerConfig_Fields(t *testing.T) {
	cfg := RunnerConfig{
		MaxScopeDepth: 5,
	}
	if cfg.MaxScopeDepth != 5 {
		t.Error("field not set")
	}
}

type computePassProbe struct {
	name   string
	called int
	value  any
}

func (p *computePassProbe) Name() string { return p.name }

func (p *computePassProbe) Run(_ *cfg.Graph, _ map[cfg.Point]*scope.State) any {
	p.called++
	return p.value
}

func TestRunnerRunComputePasses_NoGraphOrNoPasses(t *testing.T) {
	r := NewRunner(RunnerConfig{})
	if extras := r.runComputePasses(nil, nil); extras != nil {
		t.Fatalf("expected nil extras for nil graph, got %#v", extras)
	}
	if extras := r.runComputePasses(&cfg.Graph{}, nil); extras != nil {
		t.Fatalf("expected nil extras for empty pass list, got %#v", extras)
	}
}

func TestRunnerRunComputePasses_CollectsResults(t *testing.T) {
	p1 := &computePassProbe{name: "p1", value: 42}
	p2 := &computePassProbe{name: "p2", value: "ok"}
	r := NewRunner(RunnerConfig{
		ComputePasses: []api.ComputePass{p1, p2},
	})

	extras := r.runComputePasses(&cfg.Graph{}, map[cfg.Point]*scope.State{
		1: scope.New(),
	})
	if p1.called != 1 || p2.called != 1 {
		t.Fatalf("expected passes to run once each, got p1=%d p2=%d", p1.called, p2.called)
	}
	if got := extras["p1"]; got != 42 {
		t.Fatalf("expected extras[p1]=42, got %#v", got)
	}
	if got := extras["p2"]; got != "ok" {
		t.Fatalf("expected extras[p2]=ok, got %#v", got)
	}
}
