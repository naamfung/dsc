package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestExecutorCallMethod tests that exec:call and exec:async methods work correctly.
// Issue: exec:call() returns "expected function, got never"
func TestExecutorCallMethod(t *testing.T) {
	// Executor interface with call and async methods
	executorType := typ.NewInterface("exec.Executor", []typ.Method{
		{Name: "call", Type: typ.Func().
			Param("self", typ.Self).
			Param("method", typ.String).
			Variadic(typ.Any).
			Returns(typ.Any, typ.NewOptional(typ.String)).
			Build()},
		{Name: "async", Type: typ.Func().
			Param("self", typ.Self).
			Param("method", typ.String).
			Variadic(typ.Any).
			Returns(typ.Any).
			Build()},
	})

	execManifest := io.NewManifest("exec")
	execManifest.SetExport(typ.NewRecord().
		Field("get_executor", typ.Func().Returns(executorType).Build()).
		Build())
	execManifest.DefineType("Executor", executorType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "exec_call_basic",
			code: `
local executor = exec.get_executor()
local result, err = executor:call("mod:fn", "x")
`,
			wantError: false,
		},
		{
			name: "exec_call_with_multiple_args",
			code: `
local executor = exec.get_executor()
local result, err = executor:call("mod:fn", "a", "b", 123)
`,
			wantError: false,
		},
		{
			name: "exec_async_basic",
			code: `
local executor = exec.get_executor()
local handle = executor:async("mod:fn", "x")
`,
			wantError: false,
		},
		{
			name: "exec_call_with_type_annotation",
			code: `
local executor = exec.get_executor()
local result, err = executor:call("mod:fn", "x")
`,
			wantError: false,
		},
		{
			name: "exec_passed_as_param",
			code: `
local function run_call(e: exec.Executor)
    local result, err = e:call("mod:fn", "x")
    return result, err
end
`,
			wantError: false,
		},
		{
			name: "exec_stored_in_table_then_call",
			code: `
local state = { exec = nil }
state.exec = exec.get_executor()
if state.exec then
    local result, err = state.exec:call("mod:fn", "x")
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("exec", execManifest))

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

// TestRegistryVersionSnapshot tests narrowing of variables assigned in loops.
// Issue: test_version:id() becomes never after loop assignment
func TestRegistryVersionSnapshot(t *testing.T) {
	// Version interface with id and version methods
	versionType := typ.NewInterface("registry.Version", []typ.Method{
		{Name: "id", Type: typ.Func().Returns(typ.String).Build()},
		{Name: "version", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	registryManifest := io.NewManifest("registry")
	registryManifest.SetExport(typ.NewRecord().
		Field("get_versions", typ.Func().Returns(typ.NewArray(versionType)).Build()).
		Build())
	registryManifest.DefineType("Version", versionType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "version_assigned_in_loop_then_method_call",
			code: `
local versions = registry.get_versions()

local test_version
for _, v in ipairs(versions) do
    if v:version() > 0 then
        test_version = v
    end
end

if test_version then
    local id = test_version:id()
end
`,
			wantError: false,
		},
		{
			name: "version_find_first_match",
			code: `
local versions = registry.get_versions()

local found
for _, v in ipairs(versions) do
    if v:version() == 1 then
        found = v
        break
    end
end

if found then
    local id = found:id()
end
`,
			wantError: false,
		},
		{
			name: "version_find_last_match",
			code: `
local versions = registry.get_versions()

local last
for _, v in ipairs(versions) do
    last = v
end

if last then
    local id = last:id()
end
`,
			wantError: false,
		},
		{
			name: "version_conditional_assign_without_annotation",
			code: `
local versions = registry.get_versions()
local test_version = nil
for _, v in ipairs(versions) do
    if v:version() > 0 then
        test_version = v
    end
end

if test_version then
    local id = test_version:id()
end
`,
			wantError: false,
		},
		{
			name: "version_assigned_outside_loop_then_used",
			code: `
local versions = registry.get_versions()
local first = versions[1]
if first then
    local id = first:id()
end
`,
			wantError: false,
		},
		{
			name: "nested_loop_assignment",
			code: `
local versions = registry.get_versions()

local best
for _, v in ipairs(versions) do
    for i = 1, 3 do
        if v:version() > i then
            best = v
        end
    end
end

if best then
    local id = best:id()
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("registry", registryManifest))

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

// TestLoopAssignmentNarrowing tests the "never" issue when a variable starts as nil
// and gets assigned in a loop without explicit type annotation.
// TestNilInitAssignDebug traces flow for the nil->assigned->if pattern.
func TestNilInitAssignDebug(t *testing.T) {
	// Version interface with id and version methods
	versionType := typ.NewInterface("registry.Version", []typ.Method{
		{Name: "id", Type: typ.Func().Returns(typ.String).Build()},
		{Name: "version", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	registryManifest := io.NewManifest("registry")
	registryManifest.SetExport(typ.NewRecord().
		Field("get_versions", typ.Func().Returns(typ.NewArray(versionType)).Build()).
		Build())
	registryManifest.DefineType("Version", versionType)

	code := `
local versions = registry.get_versions()
local found = nil
if versions[1] then
    found = versions[1]
end
if found then
    local id = found:id()
end
`
	result := testutil.Check(code, testutil.WithStdlib(), testutil.WithManifest("registry", registryManifest))

	// Iterate over function results
	for fnExpr, funcResult := range result.Session.Results {
		if funcResult == nil || funcResult.Graph == nil {
			continue
		}
		isRoot := fnExpr == result.Session.RootFunc
		if !isRoot {
			continue // focus on main chunk
		}

		t.Logf("Function (isRoot=%v):", isRoot)

		// Log flow inputs
		if funcResult.FlowInputs != nil {
			t.Logf("  Assignments: %d", len(funcResult.FlowInputs.Assignments))
			for i, a := range funcResult.FlowInputs.Assignments {
				t.Logf("    [%d] Point=%v Target=%s Symbol=%v Type=%v", i, a.Point, a.TargetPath.Root, a.TargetPath.Symbol, a.Type)
			}

			t.Logf("  EdgeConditions: %d", len(funcResult.FlowInputs.EdgeConditions))
			for i, ec := range funcResult.FlowInputs.EdgeConditions {
				t.Logf("    Edge %d: from=%v to=%v condition=%v", i, ec.From, ec.To, ec.Condition.Disjuncts)
			}
		}

		// Log SSA graph data
		if funcResult.FlowInputs != nil && funcResult.FlowInputs.Graph != nil {
			ssaGraph := funcResult.FlowInputs.Graph
			t.Log("  SSA PhiNodes:")
			for _, phi := range ssaGraph.PhiNodes() {
				t.Logf("    Point=%v Target=%v (Key=%q, Symbol=%d) Operands:", phi.Point, phi.Target, phi.Target.Key(), phi.Target.Symbol)
				for i, op := range phi.Operands {
					t.Logf("      Operand[%d] from=%v Version=%v (Key=%q, Symbol=%d)", i, op.From, op.Version, op.Version.Key(), op.Version.Symbol)
				}
			}

			t.Log("  SSA SymbolAt and VisibleVersion for 'found':")
			for p := cfg.Point(1); p <= 20; p++ {
				sym, ok := ssaGraph.SymbolAt(p, "found")
				if ok {
					ver := ssaGraph.VisibleVersion(p, sym)
					t.Logf("    Point %v: SymbolAt=(%v, %v) VisibleVersion=%v", p, sym, ok, ver)
				}
			}

			t.Log("  SSA AllVisibleVersions:")
			for p := cfg.Point(1); p <= 20; p++ {
				vers := ssaGraph.AllVisibleVersions(p)
				if len(vers) > 0 {
					t.Logf("    Point %v: %v", p, vers)
				}
			}
		}

		// Log flow solution
		if funcResult.FlowSolution != nil {
			solution := funcResult.FlowSolution
			t.Log("  Flow types for 'found':")
			for p := cfg.Point(1); p <= 20; p++ {
				path := constraint.Path{Root: "found"}
				typeAt := solution.TypeAt(p, path)
				narrowed := solution.NarrowedTypeAt(p, path)
				cond := solution.ConditionAt(p)
				vkey := solution.DebugVersionedKey("found", p)
				if typeAt != nil || narrowed != nil || cond.HasConstraints() || vkey != "" {
					t.Logf("    Point %v: vKey=%q TypeAt=%v NarrowedTypeAt=%v Condition=%v",
						p, vkey, typeAt, narrowed, cond.AllConstraints())
				}
			}

			// Debug: show version values
			t.Log("  Version values for found@* keys:")
			for key, val := range solution.DebugVersionValues() {
				if len(key) >= 5 && key[:5] == "found" {
					t.Logf("    %s: %v", key, val)
				}
			}
		}
	}

	if result.HasError() {
		t.Errorf("got errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestLoopAssignmentNarrowing(t *testing.T) {
	// Version interface with id and version methods
	versionType := typ.NewInterface("registry.Version", []typ.Method{
		{Name: "id", Type: typ.Func().Returns(typ.String).Build()},
		{Name: "version", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	registryManifest := io.NewManifest("registry")
	registryManifest.SetExport(typ.NewRecord().
		Field("get_versions", typ.Func().Returns(typ.NewArray(versionType)).Build()).
		Build())
	registryManifest.DefineType("Version", versionType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "nil_init_loop_assign_narrowing_BUG",
			code: `
local versions = registry.get_versions()
local test_version = nil
for _, v in ipairs(versions) do
    if v:version() > 0 then
        test_version = v
    end
end
if test_version then
    local id = test_version:id()
end
`,
			wantError: false,
		},
		{
			name: "nil_init_simple_assign_then_check",
			code: `
local versions = registry.get_versions()
local v = nil
v = versions[1]
if v then
    local id = v:id()
end
`,
			wantError: false,
		},
		{
			name: "nil_init_conditional_assign_outside_loop",
			code: `
local versions = registry.get_versions()
local v = nil
if #versions > 0 then
    v = versions[1]
end
if v then
    local id = v:id()
end
`,
			wantError: false,
		},
		{
			name: "declared_nil_then_assigned_in_if",
			code: `
local versions = registry.get_versions()
local found = nil
if versions[1] then
    found = versions[1]
end
if found then
    local id = found:id()
end
`,
			wantError: false,
		},
		{
			name: "declared_with_first_value_then_reassigned",
			code: `
local versions = registry.get_versions()
local found = versions[1]
if #versions > 1 then
    found = versions[2]
end
if found then
    local id = found:id()
end
`,
			wantError: false,
		},
		{
			name: "unconditional_reassign_works",
			code: `
local versions = registry.get_versions()
local v = nil
v = versions[1]
if v then
    local id = v:id()
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("registry", registryManifest))

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

// TestTableFieldStoredMethodCall tests method calls on table fields after conditional assignment.
// Issue: result.err:kind() fails after conditional assignment
func TestTableFieldStoredMethodCall(t *testing.T) {
	// Err interface with kind and message methods
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
			name: "field_conditional_assignment_then_method_call",
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
			name: "field_assigned_from_function_return",
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
			name: "field_with_typed_record",
			code: `
local function process(flag: boolean)
    local result: { err: Err? } = { err = nil }
    if flag then
        result.err = errors.new("fail")
    end
    if result.err then
        local k = result.err:kind()
        local m = result.err:message()
    end
end
`,
			wantError: false,
		},
		{
			name: "nested_field_method_call",
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
		{
			name: "field_assigned_in_loop_then_checked",
			code: `
local items = { "a", "b", "c" }
local state = { err = nil }
for _, item in ipairs(items) do
    if item == "b" then
        state.err = errors.new("found b")
        break
    end
end
if state.err then
    local k = state.err:kind()
end
`,
			wantError: false,
		},
		{
			name: "field_cleared_then_assigned",
			code: `
local function process()
    local state: { err: Err? } = { err = errors.new("initial") }
    state.err = nil
    state.err = errors.new("new error")
    if state.err then
        local k = state.err:kind()
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
