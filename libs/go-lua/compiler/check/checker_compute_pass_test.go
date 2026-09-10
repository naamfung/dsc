package check

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/query/core"
)

type computePassProbe struct {
	name   string
	called int
}

func (p *computePassProbe) Name() string { return p.name }

func (p *computePassProbe) Run(_ *cfg.Graph, scopes map[cfg.Point]*scope.State) any {
	p.called++
	return len(scopes)
}

func TestChecker_WithComputePass(t *testing.T) {
	pass := &computePassProbe{name: "probe"}
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()}, WithComputePass(pass))
	sess := c.Check("local x = 1", "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if pass.called == 0 {
		t.Fatal("compute pass was not called")
	}
	found := false
	for _, result := range sess.Results {
		if result == nil || result.Extras == nil {
			continue
		}
		if v, ok := result.Extras[pass.name]; ok {
			if _, ok := v.(int); !ok {
				t.Fatalf("compute pass extra has wrong type: %T", v)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("expected compute pass result in FuncResult.Extras")
	}
}
