package inference

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestParamInference_FromCallArgs_SortedKeys(t *testing.T) {
	source := `
local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

local suites = {}
suites["a"] = {}
suites["b"] = {}

local names = sorted_keys(suites)
for _, name in ipairs(names) do
    local n: string = name
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
