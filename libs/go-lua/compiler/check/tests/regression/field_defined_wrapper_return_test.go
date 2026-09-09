package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: field-defined wrapper functions must not freeze a stale
// return when they call through a mutable captured table path.
func TestFieldDefinedWrapperReturnTracksVisibleCapturedFieldWrite(t *testing.T) {
	result := testutil.Check(`
local M = {
	dep = {
		get = function()
			return nil
		end,
	},
}

function M.run()
	return M.dep.get()
end

M.dep = {
	get = function()
		return { answer = "ok" }
	end,
}

local res = M.run()
local answer: string = res.answer
return answer
`)
	if result.HasError() {
		t.Fatalf("expected field-defined wrapper to see the visible reassigned field result, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Soundness guard: the wrapper must stay nilable when the write is not
// dominating on all paths.
func TestFieldDefinedWrapperReturnRequiresDominatingVisibleWrite(t *testing.T) {
	result := testutil.Check(`
local function run(flag: boolean)
	local M = {
		dep = {
			get = function()
				return nil
			end,
		},
	}

	function M.run()
		return M.dep.get()
	end

	if flag then
		M.dep = {
			get = function()
				return { answer = "ok" }
			end,
		}
	end

	local res = M.run()
	local answer: string = res.answer
	return answer
end
`)
	if !result.HasError() {
		t.Fatalf("expected non-dominating wrapper write to remain nilable after join")
	}
}

func TestFieldDefinedWrapperReturnPreservedThroughLocalAlias(t *testing.T) {
	result := testutil.Check(`
type Res = { answer: string }

local M = {
	dep = {
		get = function()
			return nil
		end,
	},
}

function M.run()
	return M.dep.get()
end

M.dep = {
	get = function()
		return { answer = "ok" }
	end,
}

local f: fun(): Res = M.run
local res = f()
local answer: string = res.answer
return answer
`)
	if result.HasError() {
		t.Fatalf("expected aliased field-defined wrapper to preserve visible reassigned field result, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFieldDefinedWrapperReturnLocalAliasRespectsCurrentFunctionValue(t *testing.T) {
	result := testutil.Check(`
type Res = { answer: string }

local M = {
	dep = {
		get = function()
			return nil
		end,
	},
}

function M.run()
	return M.dep.get()
end

M.run = function()
	return nil
end

local f: fun(): Res = M.run
return f
`)
	if !result.HasError() {
		t.Fatalf("expected aliased wrapper value typing to respect current reassigned function value")
	}
}
