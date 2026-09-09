package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard for module patterns that return {} on "no entries" and an
// accumulated array otherwise. Return-slot joining should normalize {}|T[] to T[].
func TestReturnJoin_EmptyRecordAndArrayNormalizesToArray(t *testing.T) {
	source := `
type Model = { id: string }

local function get_all(entries): ({Model}?, string?)
	if not entries then
		return {}
	end

	local all_models = {}
	for _, _ in ipairs(entries) do
		local model = { id = "" }
		table.insert(all_models, model)
	end
	return all_models
end

local models, err = get_all({1})
if err ~= nil then
	return false
end
if models ~= nil then
	local first = models[1]
	if first ~= nil then
		local id = first.id
	end
end
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
