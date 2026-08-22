package errors

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNeverType(t *testing.T) {
	// File type with name method
	fileType := typ.NewRecord().
		Field("name", typ.Func().Returns(typ.String).Build()).
		Build()

	fileManifest := io.NewManifest("file")
	fileManifest.Export = typ.NewRecord().
		Field("open", typ.Func().
			Param("path", typ.String).
			Returns(typ.NewOptional(fileType), typ.NewOptional(typ.String)).
			Build()).
		Build()

	// Create assert manifest with refinement
	notNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.NotNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)
	assertNotNil := typ.Func().
		Param("value", typ.Any).
		OptParam("msg", typ.String).
		WithRefinement(notNilEffect).
		Build()

	isNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.IsNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)
	assertIsNil := typ.Func().
		Param("value", typ.Any).
		OptParam("msg", typ.String).
		WithRefinement(isNilEffect).
		Build()

	assertType := typ.NewRecord().
		Field("not_nil", assertNotNil).
		Field("is_nil", assertIsNil).
		Build()

	assertManifest := io.NewManifest("assert")
	assertManifest.SetExport(assertType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "guard_with_not_f_then_return",
			code: `
local f: {name: fun(): string}?
if not f then return end
local n = f:name()
return n
`,
			wantError: false,
		},
		{
			name: "guard_with_f_eq_nil_then_return",
			code: `
local f: {name: fun(): string}?
if f == nil then return end
local n = f:name()
return n
`,
			wantError: false,
		},
		{
			name: "guard_with_not_f_then_error",
			code: `
local f: {name: fun(): string}?
if not f then error("f is nil") end
local n = f:name()
return n
`,
			wantError: false,
		},
		{
			name: "reassignment_after_is_nil_assertion",
			code: `
local function maybe_nil(): {name: fun(): string}?
    return nil
end

local function maybe_non_nil(): {name: fun(): string}
    return {name = function() return "test" end}
end

local f = maybe_nil()
assert.is_nil(f)
f = maybe_non_nil()
assert.not_nil(f)
local n = f:name()
return n
`,
			wantError: false,
		},
		{
			name: "file_open_with_guard",
			code: `
local f, err = file.open("test.txt")
if not f then return nil, err end
local n = f:name()
return n
`,
			wantError: false,
		},
		{
			name: "file_open_with_nil_check",
			code: `
local f, err = file.open("test.txt")
if f == nil then return nil, err end
local n = f:name()
return n
`,
			wantError: false,
		},
		{
			name: "nested_guard_pattern",
			code: `
local f: {name: fun(): string}?
if f then
    local n = f:name()
    return n
end
return nil
`,
			wantError: false,
		},
		{
			name: "guard_in_while_loop",
			code: `
local f: {name: fun(): string}?
while f do
    local n = f:name()
    return n
end
return nil
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code,
				testutil.WithStdlib(),
				testutil.WithManifest("file", fileManifest),
				testutil.WithManifest("assert", assertManifest))

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

func TestNeverType_MethodCallAfterErrorCheck(t *testing.T) {
	// Pattern from executor.lua: multiple method calls with error checks
	// After assert.is_nil(err), subsequent calls should NOT become never
	executorType := typ.NewInterface("Executor", []typ.Method{
		{Name: "call", Type: typ.Func().Param("self", typ.Self).Param("name", typ.String).Variadic(typ.Any).Returns(typ.Any, typ.NewOptional(typ.LuaError)).Build()},
		{Name: "async", Type: typ.Func().Param("self", typ.Self).Param("name", typ.String).Variadic(typ.Any).Returns(typ.Any, typ.NewOptional(typ.LuaError)).Build()},
	})

	funcsManifest := io.NewManifest("funcs")
	funcsManifest.Export = typ.NewRecord().
		Field("new", typ.Func().Returns(executorType).Build()).
		Build()

	source := `
		local exec = funcs.new()

		local result, err = exec:call("test:echo", "hello")
		if err ~= nil then error("first call no error") end
		if result == nil then error("first call has result") end

		local result2, err2 = exec:call("test:echo", "world")
		if err2 ~= nil then error("second call no error") end
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("funcs", funcsManifest))
	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d col %d: %s", d.Position.Line, d.Position.Column, d.Message)
			}
		}
		t.Errorf("expected no errors, got errors")
	}
}

func TestNeverType_LoopWithConditionalReturn(t *testing.T) {
	// Pattern from snapshot_at.lua: loop with conditional return, then method call
	versionType := typ.NewInterface("Version", []typ.Method{
		{Name: "id", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})

	registryManifest := io.NewManifest("registry")
	registryManifest.Export = typ.NewRecord().
		Field("versions", typ.Func().Returns(typ.NewArray(versionType), typ.NewOptional(typ.LuaError)).Build()).
		Field("snapshot_at", typ.Func().Param("id", typ.Number).Returns(typ.Any, typ.NewOptional(typ.LuaError)).Build()).
		Build()

	source := `
		local versions, err = registry.versions()
		if err ~= nil then error("versions no error") end

		local test_version = nil
		for _, v in ipairs(versions) do
			if v:id() > 0 then
				test_version = v
				break
			end
		end

		if test_version then
			local test_id = test_version:id()
			local snap, snap_err = registry.snapshot_at(test_id)
			if snap_err ~= nil then error("snapshot_at no error") end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("registry", registryManifest))
	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d col %d: %s", d.Position.Line, d.Position.Column, d.Message)
			}
		}
		t.Errorf("expected no errors, got errors")
	}
}

func TestNeverType_FunctionReturnInLoop(t *testing.T) {
	// Pattern from runner.lua: function returns count, used in arithmetic
	// run_suite returns #tests, but flow sees potential nil
	source := `
		local function run_suite(tests: string[]): integer, string[]
			local failures = {}
			for _, test in ipairs(tests) do
				if test == "fail" then
					table.insert(failures, test)
				end
			end
			return #tests, failures
		end

		local completed = 0
		local suites = {{"a", "b"}, {"c", "d"}}
		for _, suite in ipairs(suites) do
			local count, failures = run_suite(suite)
			completed = completed + count
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d col %d: %s", d.Position.Line, d.Position.Column, d.Message)
			}
		}
		t.Errorf("expected no errors, got errors")
	}
}

func TestNeverType_ElseAfterGuard(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "else_branch_after_return_guard",
			code: `
local f: {name: fun(): string}?
if not f then
    return nil
else
    local n = f:name()
    return n
end
`,
			wantError: false,
		},
		{
			name: "else_branch_after_error_guard",
			code: `
local f: {name: fun(): string}?
if not f then
    error("f is nil")
else
    local n = f:name()
    return n
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib())

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
