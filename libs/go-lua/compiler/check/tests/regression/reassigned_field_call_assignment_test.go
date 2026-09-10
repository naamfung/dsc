package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: when a reassigned field call synthesizes a precise
// single-value result, local assignment must preserve that result type.
func TestReassignedFieldCallAssignmentPreservesDirectResult(t *testing.T) {
	result := testutil.Check(`
local M = {
	dep = {
		get = function()
			return nil
		end,
	},
}

M.dep = {
	get = function()
		return { answer = "ok" }
	end,
}

local res = M.dep.get()
local answer: string = res.answer
return answer
`)
	if result.HasError() {
		t.Fatalf("expected direct call assignment to preserve reassigned field result, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Soundness guard: a field write from a non-dominating branch must not be
// treated as definitely visible after the join.
func TestReassignedFieldCallAssignmentRequiresDominatingWrite(t *testing.T) {
	result := testutil.Check(`
local function run(flag: boolean)
	local M = {
		dep = {
			get = function()
				return nil
			end,
		},
	}

	if flag then
		M.dep = {
			get = function()
				return { answer = "ok" }
			end,
		}
	end

	local res = M.dep.get()
	local answer: string = res.answer
	return answer
end
`)
	if !result.HasError() {
		t.Fatalf("expected non-dominating branch write to remain nilable after join")
	}
}
