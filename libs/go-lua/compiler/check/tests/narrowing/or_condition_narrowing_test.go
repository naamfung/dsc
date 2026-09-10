package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestOrConditionNarrowing_OptionalMethodCall tests that optional narrowing
// from an outer `if err then` guard persists through an inner `if` that
// uses a compound `or` condition.
//
// Reproduces E0021 "cannot call method on optional value without nil check"
// seen in wippy linter on activity_error_workflow.lua:
//
//	local result, err = funcs.call(...)
//	if err then
//	    if type(err) == "userdata" or type(err) == "table" then
//	        local m = err:message()   -- E0021 (false positive)
//	    end
//	end
//
// BUG SUMMARY (from test results):
// WORKS:
//   - `type(err) == X or type(err) == Y` (exactly 2 disjuncts, both reference err)
//   - Single field check after OR
//   - `flag and (type(err) or type(err))` (OR wrapped in AND)
//
// BROKEN (these patterns lose the outer `if err then` narrowing):
//   - Triple OR: `type(err) or type(err) or type(err)`
//   - Any disjunct not referencing err: `a or b`, `type(err) or flag`
//   - Sequential code after OR block: `if OR then end; err:method()`
//   - Else branch of OR: gets `never` type
//   - Loop inside OR block
//   - Multiple sequential field checks after OR
func TestOrConditionNarrowing_OptionalMethodCall(t *testing.T) {
	errType := typ.NewInterface("Err", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
		{Name: "message", Type: typ.Func().Returns(typ.String).Build()},
		{Name: "retryable", Type: typ.Func().Returns(typ.Boolean).Build()},
	})

	errManifest := io.NewManifest("errors")
	errManifest.SetExport(typ.NewRecord().
		Field("new", typ.Func().Param("msg", typ.String).Returns(errType).Build()).
		Build())
	errManifest.DefineType("Err", errType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		// Baseline: simple nil check then method call (already works)
		{
			name: "baseline_simple_if_err",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    local m = err:message()
end
`,
			wantError: false,
		},

		// Nested if with simple boolean condition (already works)
		{
			name: "nested_if_simple_flag",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
local flag = true
if err then
    if flag then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// Nested if with single type() check
		{
			name: "nested_if_single_type_check",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// THE BUG: nested if with compound OR type check
		{
			name: "nested_if_or_type_check_method_call",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// OR condition with multiple method calls
		{
			name: "nested_if_or_type_check_multiple_methods",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        local k = err:kind()
        local m = err:message()
        local r = err:retryable()
    end
end
`,
			wantError: false,
		},

		// OR condition with field check inside (the exact activity_error_workflow pattern)
		{
			name: "or_type_check_with_field_guard",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        if err.retryable then
            local r = err:retryable()
        end
        if err.message then
            local m = err:message()
        end
    end
end
`,
			wantError: false,
		},

		// OR condition with two identifiers (not type checks)
		{
			name: "or_condition_two_flags",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
local a = true
local b = false
if err then
    if a or b then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// AND condition (should work, since AND produces constraints)
		{
			name: "and_condition_type_checks",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if err and type(err) == "table" then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// Negated OR in else branch
		{
			name: "negated_or_else_branch",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function process()
    local val, err = get()
    if err then
        if not (type(err) == "userdata" or type(err) == "table") then
            return nil, "unknown error type"
        end
        local m = err:message()
    end
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("errors", errManifest))

			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: [%s] %s", d.Position.Line, d.Code.Name(), d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}

// TestOrConditionNarrowing_IsolatedCases tests specific patterns to isolate
// the exact narrowing failure mode.
func TestOrConditionNarrowing_IsolatedCases(t *testing.T) {
	errType := typ.NewInterface("Err", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
		{Name: "message", Type: typ.Func().Returns(typ.String).Build()},
		{Name: "retryable", Type: typ.Func().Returns(typ.Boolean).Build()},
	})

	errManifest := io.NewManifest("errors")
	errManifest.SetExport(typ.NewRecord().
		Field("new", typ.Func().Param("msg", typ.String).Returns(errType).Build()).
		Build())
	errManifest.DefineType("Err", errType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		// OR with type(x) == checks WORKS - both reference err
		{
			name: "or_type_checks_on_same_var",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// OR with unrelated variables FAILS
		{
			name: "or_unrelated_vars",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
local a = true
local b = false
if err then
    if a or b then
        local m = err:message()
    end
end
`,
			wantError: false, // Currently fails - this tests the bug
		},

		// OR with one related, one unrelated
		{
			name: "or_mixed_related_unrelated",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
local flag = true
if err then
    if type(err) == "table" or flag then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// Three levels: if err -> if (type check OR) -> if field.check (single)
		{
			name: "three_levels_or_then_single_field",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        if err.kind then
            local k = err:kind()
        end
    end
end
`,
			wantError: false,
		},

		// Three levels with TWO field checks (the failing pattern)
		{
			name: "three_levels_or_then_two_fields",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        if err.kind then
            local k = err:kind()
        end
        if err.message then
            local m = err:message()
        end
    end
end
`,
			wantError: false,
		},

		// Three levels: if err -> if flag -> if flag (no OR)
		{
			name: "three_levels_no_or",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
local a = true
local b = true
if err then
    if a then
        if b then
            local m = err:message()
        end
    end
end
`,
			wantError: false,
		},

		// Single type() check (AND form) - should work
		{
			name: "single_type_check_and",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" and err.kind then
        local k = err:kind()
    end
end
`,
			wantError: false,
		},

		// Field check directly after err check (2 levels)
		{
			name: "two_levels_field_check",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if err.kind then
        local k = err:kind()
    end
end
`,
			wantError: false,
		},

		// Minimal OR reproducer - just true or false
		{
			name: "or_literal_true_false",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if true or false then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// Sequential if statements after OR (not nested)
		{
			name: "sequential_ifs_after_or",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        local k = err:kind()
    end
    local m = err:message()
end
`,
			wantError: false,
		},

		// Multiple method calls then field check
		{
			name: "methods_then_field_check",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        local k = err:kind()
        local m = err:message()
        if err.retryable then
            local r = err:retryable()
        end
    end
end
`,
			wantError: false,
		},

		// Field check first, then method calls
		{
			name: "field_check_then_methods",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        if err.kind then
            local k = err:kind()
            local m = err:message()
        end
    end
end
`,
			wantError: false,
		},

		// Four levels deep
		{
			name: "four_levels_deep",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
local flag = true
if err then
    if type(err) == "userdata" or type(err) == "table" then
        if err.kind then
            if flag then
                local k = err:kind()
            end
        end
    end
end
`,
			wantError: false,
		},

		// OR inside AND
		{
			name: "or_inside_and",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
local flag = true
if err then
    if flag and (type(err) == "userdata" or type(err) == "table") then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// AND inside OR
		{
			name: "and_inside_or",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
local a, b = true, true
if err then
    if (a and b) or type(err) == "table" then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// Triple OR
		{
			name: "triple_or",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" or type(err) == "string" then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// OR with function call
		{
			name: "or_with_function_call",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function check(): boolean
    return true
end

local val, err = get()
if err then
    if check() or type(err) == "table" then
        local m = err:message()
    end
end
`,
			wantError: false,
		},

		// Else branch after OR
		{
			name: "else_branch_after_or",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        local m = err:message()
    else
        local k = err:kind()
    end
end
`,
			wantError: false,
		},

		// Early return in OR branch
		{
			name: "early_return_in_or_branch",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function process()
    local val, err = get()
    if err then
        if type(err) == "userdata" or type(err) == "table" then
            if err.kind then
                return err:kind()
            end
            return err:message()
        end
    end
    return val
end
`,
			wantError: false,
		},

		// Loop inside OR block
		{
			name: "loop_inside_or_block",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        for i = 1, 3 do
            local m = err:message()
        end
    end
end
`,
			wantError: false,
		},

		// Assignment inside OR block
		{
			name: "assignment_inside_or_block",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
local msg = ""
if err then
    if type(err) == "userdata" or type(err) == "table" then
        msg = err:message()
        if err.kind then
            msg = msg .. " (" .. err:kind() .. ")"
        end
    end
end
`,
			wantError: false,
		},

		// Reassignment after OR check
		{
			name: "reassignment_after_or_check",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    if type(err) == "userdata" or type(err) == "table" then
        local m = err:message()
    end
end
val, err = get()
if err then
    local k = err:kind()
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("errors", errManifest))

			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: [%s] %s", d.Position.Line, d.Code.Name(), d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}

// TestOrConditionNarrowing_FuncsCallPattern tests the exact pattern from
// wippy's activity_error_workflow using the funcs module manifest.
func TestOrConditionNarrowing_FuncsCallPattern(t *testing.T) {
	funcsManifest := testutil.FuncsManifest()

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "funcs_call_simple_err_check",
			code: `
local funcs = require("funcs")
local result, err = funcs.call("some_activity", {})
if err then
    local m = err:message()
end
`,
			wantError: false,
		},
		{
			name: "funcs_call_or_type_check",
			code: `
local funcs = require("funcs")
local result, err = funcs.call("some_activity", {})
if err then
    if type(err) == "userdata" or type(err) == "table" then
        local m = err:message()
    end
end
`,
			wantError: false,
		},
		{
			name: "funcs_call_full_activity_error_pattern",
			code: `
local funcs = require("funcs")
local result, err = funcs.call("echo_activity", {
    error_kind = "NotFound",
    error_message = "resource not found"
})

local error_kind = nil
local error_retryable = nil
local error_message = nil

if err then
    if type(err) == "userdata" or type(err) == "table" then
        if err.kind then
            error_kind = err:kind()
        end
        if err.retryable then
            error_retryable = err:retryable()
        end
        if err.message then
            error_message = err:message()
        end
    else
        error_message = tostring(err)
    end
end

return {
    activity_result = result,
    error_kind = error_kind,
    error_retryable = error_retryable,
    error_message = error_message,
}
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("funcs", funcsManifest))

			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: [%s] %s", d.Position.Line, d.Code.Name(), d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}
