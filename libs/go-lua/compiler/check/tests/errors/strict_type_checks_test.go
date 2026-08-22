package errors

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

func TestStrictTypeChecks_FieldAndReturn(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantCode diag.Code
		contains string
	}{
		{
			name: "unknown field via string index on closed record",
			code: `
local p: {name: string} = {name = "a"}
local v = p["unknown"]
`,
			wantCode: diag.ErrNoField,
			contains: "unknown",
		},
		{
			name: "index primitive number",
			code: `
local x: number = 42
local y = x[1]
`,
			wantCode: diag.ErrTypeMismatch,
			contains: "cannot index type number",
		},
		{
			name: "dot access on primitive number",
			code: `
local x: number = 42
local y = x.field
`,
			wantCode: diag.ErrTypeMismatch,
			contains: "cannot index type number",
		},
		{
			name: "type name shadowed by local value does not resolve Type:is",
			code: `
type Point = {x: number, y: number}
local Point: number = 1
local v: any = {x = 1, y = 2}
local p = Point:is(v)
`,
			wantCode: diag.ErrNoMethod,
			contains: "no method is",
		},
		{
			name: "wrong return type in branch",
			code: `
local function f(): number
	if true then
		return "nope"
	end
	return 1
end
`,
			wantCode: diag.ErrTypeMismatch,
			contains: "cannot return",
		},
		{
			name: "missing return in branch",
			code: `
local function f(): number
	if true then
		return 1
	end
end
`,
			wantCode: diag.ErrMissingReturn,
			contains: "",
		},
		{
			name: "missing return without statement",
			code: `
local function f(): number
end
`,
			wantCode: diag.ErrMissingReturn,
			contains: "",
		},
		{
			name: "typed local without initializer requires nilable",
			code: `
local x: number
`,
			wantCode: diag.ErrTypeMismatch,
			contains: "nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib())
			requireErrorCode(t, result, tt.wantCode, tt.contains)
		})
	}
}

func requireErrorCode(t *testing.T, result *testutil.Result, code diag.Code, contains string) {
	t.Helper()

	for _, d := range result.Diagnostics {
		if d.Severity != diag.SeverityError {
			continue
		}
		if d.Code != code {
			continue
		}
		if contains != "" && !strings.Contains(d.Message, contains) {
			continue
		}
		return
	}

	msgs := testutil.ErrorMessages(result.Diagnostics)
	t.Fatalf("expected error code %s containing %q, got: %v", code.Name(), contains, msgs)
}
