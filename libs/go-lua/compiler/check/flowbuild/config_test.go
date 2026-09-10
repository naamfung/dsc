package flowbuild

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFlowContext_ZeroValue(t *testing.T) {
	var fc core.FlowContext
	if fc.Graph != nil {
		t.Error("expected nil Graph in zero value")
	}
	if fc.CheckCtx != nil {
		t.Error("expected nil CheckCtx in zero value")
	}
	if fc.Base != nil {
		t.Error("expected nil Base in zero value")
	}
	if fc.Scopes != nil {
		t.Error("expected nil Scopes in zero value")
	}
	if fc.Globals != nil {
		t.Error("expected nil Globals in zero value")
	}
	if fc.API != nil {
		t.Error("expected nil API in zero value")
	}
}

func TestFlowContext_WithFields(t *testing.T) {
	fc := core.FlowContext{
		Graph:    &cfg.Graph{},
		Scopes:   make(map[cfg.Point]*scope.State),
		CheckCtx: api.NewDeclaredEnv(api.DeclaredEnvConfig{Graph: &cfg.Graph{}}),
		Base:     &scope.State{},
		Globals:  make(map[string]typ.Type),
	}
	if fc.Graph == nil {
		t.Error("expected non-nil Graph")
	}
	if fc.CheckCtx == nil {
		t.Error("expected non-nil CheckCtx")
	}
	if fc.Base == nil {
		t.Error("expected non-nil Base")
	}
	if fc.Scopes == nil {
		t.Error("expected non-nil Scopes")
	}
	if fc.Globals == nil {
		t.Error("expected non-nil Globals")
	}
}
