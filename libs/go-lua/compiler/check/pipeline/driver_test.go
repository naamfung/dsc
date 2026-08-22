package pipeline

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestNew(t *testing.T) {
	d := New(Config{
		MaxIterations: 5,
		MaxScopeDepth: 10,
	})
	if d == nil {
		t.Fatal("expected non-nil driver")
	}
	if d.cfg.MaxIterations != 5 {
		t.Error("MaxIterations not set")
	}
	if d.cfg.MaxScopeDepth != 10 {
		t.Error("MaxScopeDepth not set")
	}
}

func TestDriver_Run_NilSession(t *testing.T) {
	d := New(Config{})
	d.Run(nil, nil)
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		MaxIterations: 3,
		MaxScopeDepth: 8,
		EmitScopeDiag: true,
		GlobalTypes:   map[string]typ.Type{"foo": typ.String},
	}
	if cfg.MaxIterations != 3 {
		t.Error("MaxIterations not set")
	}
	if cfg.MaxScopeDepth != 8 {
		t.Error("MaxScopeDepth not set")
	}
	if !cfg.EmitScopeDiag {
		t.Error("EmitScopeDiag not set")
	}
	if cfg.GlobalTypes["foo"] != typ.String {
		t.Error("GlobalTypes not set")
	}
}

func TestCollectGlobalNames(t *testing.T) {
	globals := map[string]typ.Type{
		"print": typ.Any,
		"error": typ.Any,
	}
	names := collectGlobalNames(globals)
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}
	if !found["print"] {
		t.Error("print not found")
	}
	if !found["error"] {
		t.Error("error not found")
	}
}
