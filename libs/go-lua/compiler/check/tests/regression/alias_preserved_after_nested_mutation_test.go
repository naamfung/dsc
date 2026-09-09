package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestAliasPreservedAfterNestedMutationReturn(t *testing.T) {
	result := testutil.Check(`
type Builder = {_messages: {string}}

local function new(): Builder
    return {_messages = {}}
end

local function clone(): Builder
    local b = new()
    local msg: string = "x"
    table.insert(b._messages, msg)
    return b
end
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected nested mutation to preserve alias return type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
