package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTableFieldNarrowing(t *testing.T) {
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
			name: "field_conditional_assignment",
			code: `
local function process(flag: boolean)
    local result = { err = nil }
    if flag then
        result.err = errors.new("fail")
    end
    if result.err then
        local k = result.err:kind()
    end
end
`,
			wantError: false,
		},
		{
			name: "field_direct_assignment_then_check",
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
			name: "field_simple_truthy_check",
			code: `
local function process()
    local result: { err: Err? } = { err = errors.new("fail") }
    if result.err then
        local k = result.err:kind()
    end
end
`,
			wantError: false,
		},
		{
			name: "field_neq_nil_check",
			code: `
local function process()
    local result: { err: Err? } = { err = errors.new("fail") }
    if result.err ~= nil then
        local k = result.err:kind()
    end
end
`,
			wantError: false,
		},
		{
			name: "nested_field_narrowing",
			code: `
local function process(flag: boolean)
    local data: { result: { err: Err? } } = { result = { err = nil } }
    if flag then
        data.result.err = errors.new("fail")
    end
    if data.result.err then
        local k = data.result.err:kind()
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
						t.Logf("error at line %d: %s", d.Position.Line, d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}
