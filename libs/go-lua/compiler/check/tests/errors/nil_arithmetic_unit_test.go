package errors

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestNilArithmetic_ReturnTuple tests explicit return type annotation.
func TestNilArithmetic_ReturnTuple(t *testing.T) {
	source := `
		local function run_suite(): (number, number)
			return 10, 2
		end
		local completed_tests = 0
		local count, failures = run_suite()
		completed_tests = completed_tests + count
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestNilArithmetic_MultiReturnInference tests that multi-return functions
// properly infer return types for arithmetic operations.
//
// Pattern from wippy's test_runner.lua:
//
//	local count, failures = run_suite(...)
//	completed_tests = completed_tests + count  -- E0019: count inferred as nil
//
// The function returns `#tests, failures` where #tests is number.
// Type inference should propagate number type to the first return value.
func TestNilArithmetic_MultiReturnInference(t *testing.T) {
	source := `
		local function run_suite(tests: {any})
			local failures = {}
			return #tests, failures
		end

		local completed = 0
		local count, failures = run_suite({1, 2, 3})
		completed = completed + count
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors, multi-return should infer number for #tests")
	}
}

// TestNilArithmetic_SingleReturnInference tests single return inference.
func TestNilArithmetic_SingleReturnInference(t *testing.T) {
	source := `
		local function count_items(items: {any})
			return #items
		end

		local total = 0
		local n = count_items({1, 2, 3})
		total = total + n
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors, single return should infer number for #items")
	}
}

// TestNilArithmetic_LoopAccumulator tests arithmetic in loop with multi-return.
// This is the exact pattern from wippy's test_runner.lua.
func TestNilArithmetic_LoopAccumulator(t *testing.T) {
	source := `
		local function process(items: {any})
			return #items, {}
		end

		local completed = 0
		local suites = {"a", "b", "c"}

		for _, name in ipairs(suites) do
			local count, failures = process({1, 2})
			completed = completed + count
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors in loop accumulator pattern")
	}
}

// TestNilArithmetic_WippyExactPattern reproduces the exact pattern from wippy's test_runner.lua.
// Local function inside another function, with for loop accumulator.
func TestNilArithmetic_WippyExactPattern(t *testing.T) {
	source := `
		local function run_tests()
			local function run_suite(name: string, tests: {any})
				local failures = {}
				return #tests, failures
			end

			local suites = {
				{"test1", "test2", "test3"},
				{"test4", "test5"},
			}

			local completed_tests = 0

			for _, tests in ipairs(suites) do
				local count, failures = run_suite("suite", tests)
				completed_tests = completed_tests + count
			end

			return completed_tests
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors in wippy exact pattern")
	}
}

// TestNilArithmetic_TableIndexArg tests the pattern where the argument comes from table indexing.
func TestNilArithmetic_TableIndexArg(t *testing.T) {
	source := `
		local function process(items: {any})
			return #items, {}
		end

		local data: {[string]: {any}} = {
			a = {1, 2, 3},
			b = {4, 5},
		}

		local completed = 0
		local count, failures = process(data["a"])
		completed = completed + count
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors with table index argument")
	}
}

// TestNilArithmetic_UntypedParameter tests multi-return with untyped parameter.
// This is the exact pattern from wippy where run_suite has no type annotations.
func TestNilArithmetic_UntypedParameter(t *testing.T) {
	source := `
		local function run_suite(name, tests)
			local failures = {}
			return #tests, failures
		end

		local completed = 0
		local count, failures = run_suite("test", {1, 2, 3})
		completed = completed + count
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors with untyped parameter")
	}
}

// TestNilArithmetic_GroupedTablePattern tests the pattern where data comes from
// grouping functions that build tables dynamically.
func TestNilArithmetic_GroupedTablePattern(t *testing.T) {
	source := `
		local function group_entries(entries)
			local groups = {}
			for _, entry in ipairs(entries) do
				local key = "default"
				groups[key] = groups[key] or {}
				table.insert(groups[key], entry)
			end
			return groups
		end

		local function run_group(name, items)
			return #items, {}
		end

		local entries = {1, 2, 3, 4, 5}
		local groups = group_entries(entries)

		local completed = 0
		for name, items in pairs(groups) do
			local count, failures = run_group(name, items)
			completed = completed + count
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors in grouped table pattern")
	}
}

// TestNilArithmetic_LengthOfAny tests that #any returns integer, not nil.
// This is the root cause of wippy's E0019 error.
// registry.find() returns any[], and after grouping, suites[name] is any.
// run_suite(tests) where tests is any, then #tests should be integer.
func TestNilArithmetic_LengthOfAny(t *testing.T) {
	source := `
		local function run_suite(tests: any)
			return #tests, {}
		end

		local completed = 0
		local count, failures = run_suite({1, 2, 3})
		completed = completed + count
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors: #any should return integer, not nil")
	}
}

// TestNilArithmetic_RegistryPattern reproduces the exact wippy pattern.
// registry.find returns (any[], error?), entries flow through group_by_suite,
// suites[name] is any, and #tests on any parameter returns nil.
func TestNilArithmetic_RegistryPattern(t *testing.T) {
	source := `
		-- Simulates registry.find return type: (any[], error?)
		local function registry_find(): ({any}, string?)
			return {{id = "test1"}, {id = "test2"}}, nil
		end

		local function group_by_suite(entries: {any})
			local suites: {[string]: {any}} = {}
			for _, entry in ipairs(entries) do
				local suite = "default"
				suites[suite] = suites[suite] or {}
				table.insert(suites[suite], entry)
			end
			return suites
		end

		local function run_suite(name, tests, completed, total)
			local failures = {}
			return #tests, failures
		end

		local entries, err = registry_find()
		local suites = group_by_suite(entries)
		local suite_names = {"default"}

		local completed_tests = 0
		local total_tests = 2

		for idx, name in ipairs(suite_names) do
			local count, failures = run_suite(name, suites[name], completed_tests, total_tests)
			completed_tests = completed_tests + count
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors in registry pattern")
	}
}

// TestNilArithmetic_UntypedGrouping tests the exact wippy pattern without any type annotations.
// This is the closest match to how runner.lua actually works.
func TestNilArithmetic_UntypedGrouping(t *testing.T) {
	source := `
		local function group_by_suite(entries)
			local suites = {}
			local no_suite = {}
			for _, entry in ipairs(entries) do
				local suite = entry.meta and entry.meta.suite
				if suite then
					suites[suite] = suites[suite] or {}
					table.insert(suites[suite], entry)
				else
					table.insert(no_suite, entry)
				end
			end
			return suites, no_suite
		end

		local function run_suite(name, tests, suite_idx, total_suites, completed_tests, total_tests)
			local failures = {}
			return #tests, failures
		end

		local entries = {{id = "a", meta = {suite = "s1"}}, {id = "b", meta = {suite = "s1"}}}
		local suites, no_suite = group_by_suite(entries)
		local suite_names = {"s1"}

		local completed_tests = 0
		local total_tests = 2
		local total_suites = 1

		for idx, name in ipairs(suite_names) do
			local count, failures = run_suite(name, suites[name], idx, total_suites, completed_tests, total_tests)
			completed_tests = completed_tests + count
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors in untyped grouping pattern")
	}
}

// TestNilArithmetic_WithRegistryManifest tests using actual manifest like wippy.
// registry.find returns (any[], error?) from manifest.
func TestNilArithmetic_WithRegistryManifest(t *testing.T) {
	// Create registry manifest matching wippy's types.go
	registryManifest := io.NewManifest("registry")
	registryModule := typ.NewInterface("registry", []typ.Method{
		{Name: "find", Type: typ.Func().
			Param("query", typ.Any).
			Returns(typ.NewArray(typ.Any), typ.NewOptional(typ.LuaError)).
			Build()},
	})
	registryManifest.SetExport(registryModule)

	source := `
		local function group_by_suite(entries)
			local suites = {}
			local no_suite = {}
			for _, entry in ipairs(entries) do
				local suite = entry.meta and entry.meta.suite
				if suite then
					suites[suite] = suites[suite] or {}
					table.insert(suites[suite], entry)
				else
					table.insert(no_suite, entry)
				end
			end
			return suites, no_suite
		end

		local function run_suite(name, tests, suite_idx, total_suites, completed_tests, total_tests)
			local failures = {}
			return #tests, failures
		end

		local entries, err = registry.find({["meta.type"] = "test"})
		local suites, no_suite = group_by_suite(entries)
		local suite_names = {"s1"}

		local completed_tests = 0
		local total_tests = 2
		local total_suites = 1

		for idx, name in ipairs(suite_names) do
			local count, failures = run_suite(name, suites[name], idx, total_suites, completed_tests, total_tests)
			completed_tests = completed_tests + count
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("registry", registryManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors with registry manifest: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestNilArithmetic_LengthOfOptional tests that #(T?) should error or be handled.
// When suites[name] returns {any}? (optional), and tests param receives it,
// then #tests on optional type might be the issue.
func TestNilArithmetic_LengthOfOptional(t *testing.T) {
	source := `
		local function run_suite(tests)
			return #tests, {}
		end

		local suites: {[string]: {any}} = {
			s1 = {1, 2, 3}
		}

		local completed = 0
		-- suites["s1"] has type {any}? because map indexing can return nil
		local count, failures = run_suite(suites["s1"])
		completed = completed + count
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	// This test documents the current behavior - might fail if #optional returns nil
	if result.HasError() {
		t.Errorf("expected no errors with optional table index: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestNilArithmetic_MapIndexToUntyped tests the exact flow: map[key] -> untyped param -> #param
func TestNilArithmetic_MapIndexToUntyped(t *testing.T) {
	source := `
		local function process(items)
			return #items, {}
		end

		local data: {[string]: {number}} = {
			a = {1, 2, 3}
		}

		local completed = 0
		local count, failures = process(data["a"])
		completed = completed + count
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestNilArithmetic_ComplexFunctionBody tests return type inference with complex function body.
// This matches wippy's run_suite which has loops, conditionals, and multiple statements.
func TestNilArithmetic_ComplexFunctionBody(t *testing.T) {
	source := `
		local function run_suite(name, tests, suite_idx, total_suites, completed_tests, total_tests)
			local suite_passed = 0
			local suite_failed = 0
			local failures = {}

			for i, entry in ipairs(tests) do
				local ok = true
				if ok then
					suite_passed = suite_passed + 1
				else
					suite_failed = suite_failed + 1
					table.insert(failures, {name = "test", error = "failed"})
				end
			end

			return #tests, failures
		end

		local suites = {
			s1 = {{id = "t1"}, {id = "t2"}}
		}
		local suite_names = {"s1"}

		local completed_tests = 0

		for idx, name in ipairs(suite_names) do
			local count, failures = run_suite(name, suites[name], idx, 1, completed_tests, 2)
			completed_tests = completed_tests + count
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors with complex function body: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestNilArithmetic_FunctionReturnAssignedToLocal tests that function returns assigned to
// local variables preserve their types through the assignment.
func TestNilArithmetic_FunctionReturnAssignedToLocal(t *testing.T) {
	source := `
		local function group_by_suite(entries)
			local suites = {}
			for _, e in ipairs(entries) do
				local k = "default"
				suites[k] = suites[k] or {}
				table.insert(suites[k], e)
			end
			return suites
		end

		local function run_suite(tests)
			return #tests, {}
		end

		local entries = {{id = "a"}, {id = "b"}}
		local suites = group_by_suite(entries)

		local completed = 0
		for name, tests in pairs(suites) do
			local count, failures = run_suite(tests)
			completed = completed + count
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestNilArithmetic_PairsIterationValueType tests that pairs() iteration preserves value types.
func TestNilArithmetic_PairsIterationValueType(t *testing.T) {
	source := `
		local function process(items)
			return #items
		end

		local data = {
			a = {1, 2, 3},
			b = {4, 5}
		}

		local total = 0
		for key, items in pairs(data) do
			total = total + process(items)
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
