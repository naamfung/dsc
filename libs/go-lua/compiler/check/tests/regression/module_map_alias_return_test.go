package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: map-shaped type aliases exported from a module must survive
// import on function returns and through `or` fallback.
func TestModuleMapAliasReturnPreservedAcrossImport(t *testing.T) {
	contextModule := testutil.CheckAndExport(`
type Context = {[string]: any}

local M = {}
M.Context = Context

function M.empty(): Context
	return {}
end

return M
`, "context", testutil.WithStdlib())
	if contextModule.HasError() {
		t.Fatalf("unexpected context module errors: %v", testutil.ErrorMessages(contextModule.Errors))
	}

	result := testutil.Check(`
local context = require("context")

function with_default(initial: context.Context?): context.Context
	local ctx = initial or context.empty()
	return ctx
end
`,
		testutil.WithStdlib(),
		testutil.WithModule("context", contextModule),
	)
	if result.HasError() {
		t.Fatalf("expected imported map alias return to survive fallback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
