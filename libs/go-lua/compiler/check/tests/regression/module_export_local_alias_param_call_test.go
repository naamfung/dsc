package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
)

// Regression guard: exported function params that rely on module-local type
// aliases must remain structurally callable by downstream modules after
// manifest encode/decode.
func TestRegression_ModuleExport_LocalAliasParam_StructuralCall(t *testing.T) {
	producerSource := `
type AgentContextConfig = {
	enable_cache: boolean?,
	context: {[string]: any}?,
	delegate_tools: {enabled: boolean?}?,
	memory_contract: {implementation_id: string, context: {[string]: any}?}?,
	context_merger: any?,
}

local M = {}

function M.new(config: AgentContextConfig?): boolean
	config = config or {}
	return config.enable_cache == true
end

return M
`

	exported := testutil.CheckAndExport(producerSource, "agent_context", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatalf("unexpected producer errors: %v", testutil.ErrorMessages(exported.Errors))
	}

	encoded, err := io.EncodeManifest(exported.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	consumerSource := `
local agent_context = require("agent_context")
local ok: boolean = agent_context.new({
	enable_cache = true,
	context = {},
})
return ok
`

	result := testutil.Check(consumerSource,
		testutil.WithStdlib(),
		testutil.WithManifest("agent_context", decoded),
	)
	if result.HasError() {
		t.Fatalf("expected no errors for structural literal call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard for inferred (unannotated) constructor-style APIs that
// still derive a local alias parameter shape from usage.
func TestRegression_ModuleExport_InferredLocalAliasParam_StructuralCall(t *testing.T) {
	producerSource := `
type AgentContextConfig = {
	enable_cache: boolean?,
	context: {[string]: any}?,
	delegate_tools: {enabled: boolean?}?,
	memory_contract: {implementation_id: string, context: {[string]: any}?}?,
	context_merger: any?,
}

local M = {}

function M.new(config)
	config = config or {}
	local enabled = config.enable_cache ~= nil and config.enable_cache or true
	local base_context = config.context or {}
	return enabled == true and base_context ~= nil
end

return M
`

	exported := testutil.CheckAndExport(producerSource, "agent_context", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatalf("unexpected producer errors: %v", testutil.ErrorMessages(exported.Errors))
	}

	encoded, err := io.EncodeManifest(exported.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	consumerSource := `
local agent_context = require("agent_context")
local ok: boolean = agent_context.new({
	enable_cache = true,
	context = {},
})
return ok
`

	result := testutil.Check(consumerSource,
		testutil.WithStdlib(),
		testutil.WithManifest("agent_context", decoded),
	)
	if result.HasError() {
		t.Fatalf("expected no errors for inferred local alias call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
