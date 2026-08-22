package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestErrorObjectNarrowing tests that error objects are correctly narrowed
// after nil checks, allowing method calls like err:kind() and err:message().
func TestErrorObjectNarrowing(t *testing.T) {
	// Create Err type with kind() and message() methods
	errType := typ.NewInterface("Err", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
		{Name: "message", Type: typ.Func().Returns(typ.String).Build()},
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
		{
			name: "err_kind_after_if_err",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    local k = err:kind()
    local m = err:message()
end
`,
			wantError: false,
		},
		{
			name: "err_kind_after_if_err_neq_nil",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err ~= nil then
    local k = err:kind()
    local m = err:message()
end
`,
			wantError: false,
		},
		{
			name: "err_kind_after_not_err_return",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function process()
    local val, err = get()
    if not err then return end
    local k = err:kind()
    local m = err:message()
    return k, m
end
`,
			wantError: false,
		},
		{
			name: "err_kind_after_err_eq_nil_return",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function process()
    local val, err = get()
    if err == nil then return end
    local k = err:kind()
    local m = err:message()
    return k, m
end
`,
			wantError: false,
		},
		{
			name: "err_in_else_branch",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if not err then
    -- val is valid here
else
    local k = err:kind()
    local m = err:message()
end
`,
			wantError: false,
		},
		{
			name: "err_reassigned_then_checked",
			code: `
local function get1(): (string?, Err?)
    return nil, errors.new("fail1")
end
local function get2(): (string?, Err?)
    return nil, errors.new("fail2")
end

local val, err = get1()
if err then
    local k = err:kind()
end

val, err = get2()
if err then
    local k = err:kind()
end
`,
			wantError: false,
		},
		{
			name: "nested_error_checks",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function process()
    local a, err1 = get()
    if err1 then
        return nil, err1
    end

    local b, err2 = get()
    if err2 then
        local k = err2:kind()
        return nil, err2
    end

    return a .. b, nil
end
`,
			wantError: false,
		},
		{
			name: "error_method_chain",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local val, err = get()
if err then
    local msg = "Error: " .. err:kind() .. " - " .. err:message()
end
`,
			wantError: false,
		},
		{
			name: "error_as_function_arg",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function log(k: string, m: string)
end

local val, err = get()
if err then
    log(err:kind(), err:message())
end
`,
			wantError: false,
		},
		{
			name: "inline_error_type_annotation",
			code: `
local err: Err?
if err then
    local k = err:kind()
    local m = err:message()
end
`,
			wantError: false,
		},
		{
			name: "error_from_optional_return",
			code: `
local function might_fail(): Err?
    return errors.new("fail")
end

local e = might_fail()
if e then
    local k = e:kind()
    local m = e:message()
end
`,
			wantError: false,
		},
		{
			name: "second_return_value_method_call",
			code: `
local function maybe_fail(): (boolean, Err?)
    return false, errors.new("failed")
end

local ok, err = maybe_fail()
if err then
    local k = err:kind()
    local m = err:message()
end
`,
			wantError: false,
		},
		{
			name: "assert_style_guard_then_method",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function run()
    local val, err = get()
    if not err then
        return val
    end
    -- err is non-nil here
    error(err:message())
end
`,
			wantError: false,
		},
		{
			name: "error_passed_to_handler",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function handle(e: Err)
    return e:kind()
end

local val, err = get()
if err then
    handle(err)
end
`,
			wantError: false,
		},
		{
			name: "error_in_while_loop",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function process()
    local val, err = get()
    while err do
        local k = err:kind()
        val, err = get()
    end
    return val
end
`,
			wantError: false,
		},
		{
			name: "error_from_closure",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function make_handler()
    local val, err = get()
    return function()
        if err then
            return err:kind()
        end
        return nil
    end
end
`,
			wantError: false,
		},
		{
			name: "error_stored_in_table",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function process()
    local result = { err = nil }
    local val
    val, result.err = get()
    if result.err then
        local k = result.err:kind()
    end
    return val
end
`,
			wantError: false,
		},
		{
			name: "error_with_or_pattern",
			code: `
local function get1(): (string?, Err?)
    return nil, errors.new("fail1")
end
local function get2(): (string?, Err?)
    return nil, errors.new("fail2")
end

local function process()
    local v1, e1 = get1()
    local v2, e2 = get2()
    local err = e1 or e2
    if err then
        local k = err:kind()
    end
end
`,
			wantError: false,
		},
		{
			name: "error_method_in_return",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function process()
    local val, err = get()
    if err then
        return nil, err:message()
    end
    return val, nil
end
`,
			wantError: false,
		},
		{
			name: "triple_return_error",
			code: `
local function fetch(): (string?, number?, Err?)
    return nil, nil, errors.new("fail")
end

local function process()
    local a, b, err = fetch()
    if err then
        local k = err:kind()
        local m = err:message()
    end
    return a, b
end
`,
			wantError: false,
		},
		{
			name: "error_conditional_assignment",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function process(flag: boolean)
    local err: Err?
    if flag then
        local val
        val, err = get()
    end
    if err then
        local k = err:kind()
    end
end
`,
			wantError: false,
		},
		{
			name: "error_conditional_simple",
			code: `
local function process(flag: boolean)
    local err: Err?
    if flag then
        err = errors.new("fail")
    end
    if err then
        local k = err:kind()
    end
end
`,
			wantError: false,
		},
		{
			name: "error_reassign_from_nil",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function process()
    local err: Err? = nil
    local val
    val, err = get()
    if err then
        local k = err:kind()
    end
end
`,
			wantError: false,
		},
		{
			name: "error_method_in_concat",
			code: `
local function get(): (string?, Err?)
    return nil, errors.new("fail")
end

local function process()
    local val, err = get()
    if err then
        return "Error: " .. err:kind() .. " - " .. err:message()
    end
    return val
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
						t.Logf("error at line %d: %s", d.Position.Line, d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}
