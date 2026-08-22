package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
)

// Regression guard: imported function type aliases must stay callable even
// after local aliasing through a guarded map lookup, and their returned union
// should remain precise enough for discriminant narrowing.
func TestModuleFunctionAliasReturnPreservedAcrossGuardedMapLookup(t *testing.T) {
	producerModule := testutil.CheckAndExport(`
type Payload = {id: string}
type Outcome = {ok: true, value: Payload} | {ok: false, error: {message: string}}
type Handler = (string) -> Outcome

local M = {}
M.Payload = Payload
M.Outcome = Outcome
M.Handler = Handler

return M
`, "producer", testutil.WithStdlib())
	if producerModule.HasError() {
		t.Fatalf("unexpected producer errors: %v", testutil.ErrorMessages(producerModule.Errors))
	}

	encoded, err := io.EncodeManifest(producerModule.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	result := testutil.Check(`
local producer = require("producer")

type Handler = producer.Handler

local handlers: {[string]: Handler} = {
	["a"] = function(s: string)
		return {ok = true, value = {id = s}}
	end,
}

local h = handlers["a"]
if not h then
	return nil
end

local out = h("x")
if out.ok then
	local id: string = out.value.id
else
	local msg: string = out.error.message
end
`,
		testutil.WithStdlib(),
		testutil.WithManifest("producer", decoded),
	)
	if result.HasError() {
		t.Fatalf("expected imported function alias call to preserve return union, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
