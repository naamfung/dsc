package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression: computed index access with unresolved key type should not report
// "cannot index type {[string]: any}" on dynamic map-like tables.
func TestComputedIndex_UnresolvedKeyType_DynamicMap(t *testing.T) {
	result := testutil.Check(`
local content_blocks = {}
local data = {} :: any
if data.index ~= nil and data.content_block then
	content_blocks[data.index] = data.content_block
end

local index = data.index or 0
if content_blocks[index] and content_blocks[index].type == "tool_use" then
	local x = content_blocks[index].id or ""
	return x
end
return ""
`, testutil.WithStdlib())

	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
