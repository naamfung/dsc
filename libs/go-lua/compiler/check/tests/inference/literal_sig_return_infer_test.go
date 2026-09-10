package inference

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestParamTypeInference_TableNestedFunction tests that parameter types are inferred
// from usage for functions defined inside table literals.
// x > 0 implies x is number, so return type should be number | nil.
func TestParamTypeInference_TableNestedFunction(t *testing.T) {
	source := `
		local utils = {
			get_value = function(x)
				if x > 0 then
					return x
				end
				return nil
			end
		}

		local function use_number(n: number)
			print(n)
		end

		local v = utils.get_value(5)
		if v then
			use_number(v)
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
}

// TestParamTypeInference_TableNestedArithmetic tests parameter inference from
// arithmetic operations in table-nested functions.
func TestParamTypeInference_TableNestedArithmetic(t *testing.T) {
	source := `
		local math_utils = {
			double = function(x)
				return x * 2
			end
		}

		local function use_number(n: number)
			print(n)
		end

		use_number(math_utils.double(5))
	`

	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
}

// TestParamTypeInference_TableNestedStringConcat tests parameter inference from
// string concatenation in table-nested functions.
func TestParamTypeInference_TableNestedStringConcat(t *testing.T) {
	source := `
		local string_utils = {
			greet = function(name)
				return "Hello, " .. name
			end
		}

		local function use_string(s: string)
			print(s)
		end

		use_string(string_utils.greet("World"))
	`

	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
}

// TestWippyRunnerPattern reproduces the wippy test_runner issue
func TestWippyRunnerPattern(t *testing.T) {
	source := `
		local function sorted_keys(t)
			local keys = {}
			for k in pairs(t) do
				table.insert(keys, k)
			end
			table.sort(keys)
			return keys
		end

		local function run_suite(name: string, tests: {any})
			print(name)
			return #tests
		end

		-- This mimics the wippy pattern more closely
		local function group_by_suite(entries)
			local suites = {}
			for _, entry in ipairs(entries) do
				local suite = entry.meta and entry.meta.suite
				if suite then
					suites[suite] = suites[suite] or {}
					table.insert(suites[suite], entry)
				end
			end
			return suites
		end

		local entries = {
			{meta = {suite = "a"}, id = "test1"},
			{meta = {suite = "b"}, id = "test2"},
		}

		local suites = group_by_suite(entries)
		local suite_names = sorted_keys(suites)
		for idx, name in ipairs(suite_names) do
			run_suite(name, suites[name])
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		t.Logf("Diagnostic: %s", d.Message)
	}

	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Errors))
	}
}

// TestSortedKeysReturn tests that sorted_keys returns proper string array
func TestSortedKeysReturn(t *testing.T) {
	source := `
		local function sorted_keys(t)
			local keys = {}
			for k in pairs(t) do
				table.insert(keys, k)
			end
			table.sort(keys)
			return keys
		end

		local suites = {}
		suites["a"] = 1
		suites["b"] = 2

		local names = sorted_keys(suites)
		for _, name in ipairs(names) do
			local s: string = name
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if strings.Contains(d.Message, "string") {
			t.Logf("Type error: %s", d.Message)
		}
	}
}
