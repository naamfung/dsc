package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestLinterFalsePositive_PairsKeyOfDirect ensures that keys from pairs() loops
// are recognized as valid keys for indexing the same table.
func TestLinterFalsePositive_PairsKeyOfDirect(t *testing.T) {
	source := `
local suites: {[string]: {any}} = {}
suites["a"] = { 1 }
suites["b"] = { 2 }

for name in pairs(suites) do
    local v: {any} = suites[name]
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestLinterFalsePositive_PairsKeyOfWithValue ensures that both key and value
// variables in pairs() loops work correctly with table indexing.
func TestLinterFalsePositive_PairsKeyOfWithValue(t *testing.T) {
	source := `
local suites: {[string]: {any}} = {}
suites["a"] = { 1 }

for name, suite in pairs(suites) do
    local v: {any} = suites[name]
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestLinterFalsePositive_SortedKeysIndexing ensures keys derived from a table
// are treated as valid keys for indexing that table.
func TestLinterFalsePositive_SortedKeysIndexing(t *testing.T) {
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
suites["a"] = { value = 1 }
suites["b"] = { value = 2 }

local names = sorted_keys(suites)
for _, name in ipairs(names) do
    local v: number = suites[name].value
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestLinterFalsePositive_SortedKeysKeyType(t *testing.T) {
	source := `
local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    return keys
end

local t = { a = 1, b = 2 }
local names = sorted_keys(t)
for _, name in ipairs(names) do
    local k: "a" | "b" = name
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestLinterFalsePositive_MultiReturnSecondValue ensures second return values
// are not widened to nil when all returns supply them.
func TestLinterFalsePositive_MultiReturnSecondValue(t *testing.T) {
	source := `
local function group_by_suite(entries)
    local suites = {}
    local no_suite = {}
    if #entries > 0 then
        return suites, no_suite
    end
    return suites, no_suite
end

local suites, no_suite = group_by_suite({})
if #no_suite > 0 then
    local n: number = #no_suite
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestLinterFalsePositive_ReturnTuplePrecision ensures first return remains integer
// when all branches return integer as first value.
func TestLinterFalsePositive_ReturnTuplePrecision(t *testing.T) {
	source := `
local function run_suite(name, suites)
    local failures = {}
    if name == "a" then
        return 1, failures
    end
    return 2, failures
end

local completed_tests = 0
local count, failures = run_suite("a", {})
completed_tests = completed_tests + count
if #failures > 0 then
    local n: number = #failures
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestLinterFalsePositive_ComplexControlFlowReturn ensures return type inference
// handles complex control flow without widening to union when all paths return same types.
func TestLinterFalsePositive_ComplexControlFlowReturn(t *testing.T) {
	source := `
local function run_suite(name: string, tests: {}[]): (integer, {}[])
    local failures = {}
    local count = 0
    for _, test in ipairs(tests) do
        count = count + 1
        if test.skip then
            -- skip
        elseif test.fail then
            table.insert(failures, test)
        end
    end
    return count, failures
end

local completed_tests = 0
local count, failures = run_suite("suite1", {})
completed_tests = completed_tests + count
if #failures > 0 then
    local n: number = #failures
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestLinterFalsePositive_MultipleReturnsWithLengthOp ensures length operator works
// on second return value when all returns provide it.
func TestLinterFalsePositive_MultipleReturnsWithLengthOp(t *testing.T) {
	source := `
local function process(items: {}[])
    local results = {}
    local errors = {}
    for _, item in ipairs(items) do
        if item.valid then
            table.insert(results, item)
        else
            table.insert(errors, item)
        end
    end
    return results, errors
end

local results, errors = process({})
local count: integer = #results + #errors
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestLinterFalsePositive_InferredReturnNoAnnotation ensures return type inference
// works correctly without explicit annotations when all branches return consistent types.
func TestLinterFalsePositive_InferredReturnNoAnnotation(t *testing.T) {
	source := `
local function run_suite(name, tests)
    local failures = {}
    local count = 0
    for _, test in ipairs(tests) do
        count = count + 1
        if test.fail then
            table.insert(failures, test)
        end
    end
    return count, failures
end

local completed_tests = 0
local count, failures = run_suite("suite1", {})
completed_tests = completed_tests + count
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestLinterFalsePositive_TestRunnerPattern replicates the exact pattern from
// the test runner: sorted_keys iteration, run_suite with multi-return, and
// accumulating completed_tests count.
func TestLinterFalsePositive_TestRunnerPattern(t *testing.T) {
	source := `
local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

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
    for i, entry in ipairs(tests) do
        local ok = true
        if not ok then
            table.insert(failures, {
                name = entry.id,
                id = entry.id,
                error = "test failed"
            })
        end
    end
    return #tests, failures
end

local suites, no_suite = group_by_suite({})
local suite_names = sorted_keys(suites)

local total_tests = 10
local total_suites = #suite_names + (#no_suite > 0 and 1 or 0)
local completed_tests = 0

for idx, name in ipairs(suite_names) do
    local count, failures = run_suite(name, suites[name], idx, total_suites, completed_tests, total_tests)
    completed_tests = completed_tests + count
end

if #no_suite > 0 then
    local count, failures = run_suite("other", no_suite, total_suites, total_suites, completed_tests, total_tests)
    completed_tests = completed_tests + count
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestLinterFalsePositive_TestRunnerExact replicates the exact test runner
// pattern that fails in wippy.
func TestLinterFalsePositive_TestRunnerExact(t *testing.T) {
	source := `
local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

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

local function run_suite(name: string, tests: {any}, suite_idx: number, total_suites: number, completed_tests: number, total_tests: number)
    local failures = {}
    for i, entry in ipairs(tests) do
        local ok = true
        if not ok then
            table.insert(failures, {
                name = entry.id,
                id = entry.id,
                error = "test failed",
                time = 0
            })
        end
    end
    return #tests, failures
end

local suites, no_suite = group_by_suite({})
local suite_names = sorted_keys(suites)

local total_tests = 10
local total_suites = #suite_names + (#no_suite > 0 and 1 or 0)
local completed_tests = 0

for idx, name in ipairs(suite_names) do
    local count, failures = run_suite(name, suites[name], idx, total_suites, completed_tests, total_tests)
    completed_tests = completed_tests + count
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestLinterFalsePositive_TestRunnerWithTypedEntries tests with explicitly typed entries
// to better match real-world usage where entries come from a typed registry.
func TestLinterFalsePositive_TestRunnerWithTypedEntries(t *testing.T) {
	source := `
type Entry = {id: string, meta: {type: string, suite: string?, order: number?}?}

local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

local function group_by_suite(entries: {Entry})
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

local function run_suite(name: string, tests: {Entry}, suite_idx: number, total_suites: number, completed_tests: number, total_tests: number)
    local failures = {}
    for i, entry in ipairs(tests) do
        local ok = true
        if not ok then
            table.insert(failures, {
                name = entry.id,
                id = entry.id,
                error = "test failed",
                time = 0
            })
        end
    end
    return #tests, failures
end

local entries: {Entry} = {}
local suites, no_suite = group_by_suite(entries)
local suite_names = sorted_keys(suites)

local total_tests = 10
local total_suites = #suite_names + (#no_suite > 0 and 1 or 0)
local completed_tests = 0

for idx, name in ipairs(suite_names) do
    local count, failures = run_suite(name, suites[name], idx, total_suites, completed_tests, total_tests)
    completed_tests = completed_tests + count
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestLinterFalsePositive_TestRunnerWithFalseMeta mirrors registry entries that may
// use false for missing metadata (meta: table | false).
func TestLinterFalsePositive_TestRunnerWithFalseMeta(t *testing.T) {
	source := `
type Entry = {id: string, meta: {type: string, suite: string?, order: number?} | false}

local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

local function group_by_suite(entries: {Entry})
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

local function run_suite(name: string, tests: {Entry}, suite_idx: number, total_suites: number, completed_tests: number, total_tests: number)
    local failures = {}
    for i, entry in ipairs(tests) do
        local ok = true
        if not ok then
            table.insert(failures, {
                name = entry.id,
                id = entry.id,
                error = "test failed",
                time = 0
            })
        end
    end
    return #tests, failures
end

local entries: {Entry} = {}
local suites, no_suite = group_by_suite(entries)
local suite_names = sorted_keys(suites)

local total_tests = 10
local total_suites = #suite_names + (#no_suite > 0 and 1 or 0)
local completed_tests = 0

for idx, name in ipairs(suite_names) do
    local count, failures = run_suite(name, suites[name], idx, total_suites, completed_tests, total_tests)
    completed_tests = completed_tests + count
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestLinterFalsePositive_AnnotatedStringNarrowsToLiteral(t *testing.T) {
	source := `
local function expect_literal(x: string)
    if x == "a" then
        local y: "a" = x
        return y
    end
    return x
end
local _ = expect_literal("a")
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for annotated literal narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestLinterFalsePositive_AnnotatedNumberNarrowsToLiteral(t *testing.T) {
	source := `
local function expect_one(x: number): number
    if x == 1 then
        local y: 1 = x
        return y
    end
    return x
end
local _ = expect_one(1)
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for annotated numeric literal narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
