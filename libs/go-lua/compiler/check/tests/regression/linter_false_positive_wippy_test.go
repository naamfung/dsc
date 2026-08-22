package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestLinterFalsePositive_WippyTestRunner reproduces the wippy test runner
// error where count becomes a union of tuples instead of integer.
// Error: cannot perform arithmetic on (integer, {error: unknown,...}[]) | (integer, {...} | {...}[])
func TestLinterFalsePositive_WippyTestRunner(t *testing.T) {
	// Build manifests for external modules
	registryManifest := io.NewManifest("registry")
	entryType := typ.NewRecord().
		Field("id", typ.String).
		Field("name", typ.String).
		Field("meta", typ.NewOptional(typ.NewRecord().
			Field("type", typ.String).
			Field("suite", typ.NewOptional(typ.String)).
			Field("order", typ.NewOptional(typ.Number)).
			Build())).
		Build()
	registryManifest.SetExport(typ.NewRecord().
		Field("find", typ.Func().
			Param("category", typ.String).
			Returns(typ.NewArray(entryType)).
			Build()).
		Build())
	registryManifest.DefineType("Entry", entryType)

	funcsManifest := io.NewManifest("funcs")
	funcsManifest.SetExport(typ.NewRecord().
		Field("call", typ.Func().
			Param("name", typ.String).
			Variadic(typ.Any).
			Returns(typ.Any, typ.NewOptional(typ.LuaError)).
			Build()).
		Build())

	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(typ.NewRecord().
		Field("now", typ.Func().Returns(typ.Number).Build()).
		Build())

	source := `
local registry = require("registry")
local funcs = require("funcs")
local time = require("time")

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
        local start_time = time.now()
        local result, err = funcs.call(entry.name)
        local elapsed = time.now() - start_time

        if err then
            table.insert(failures, {
                name = entry.name,
                id = entry.id,
                error = tostring(err),
                time = elapsed
            })
        end
    end
    return #tests, failures
end

local entries = registry.find("test")
local suites, no_suite = group_by_suite(entries)
local suite_names = sorted_keys(suites)

local total_tests = #entries
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
	result := testutil.Check(source, testutil.WithStdlib(),
		testutil.WithManifest("registry", registryManifest),
		testutil.WithManifest("funcs", funcsManifest),
		testutil.WithManifest("time", timeManifest))

	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors in wippy test runner pattern")
	}
}

// TestLinterFalsePositive_WippyTestRunner_WithRegistryFind mirrors the real
// registry.find signature (returns entries + error) and validates the sound
// normalization pattern when suite metadata is any-typed.
func TestLinterFalsePositive_WippyTestRunner_WithRegistryFind(t *testing.T) {
	registryManifest := io.NewManifest("registry")
	entryType := typ.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewMap(typ.String, typ.Any)).
		Build()
	registryManifest.SetExport(typ.NewRecord().
		Field("find", typ.Func().
			Param("query", typ.Any).
			Returns(typ.NewArray(entryType), typ.NewOptional(typ.LuaError)).
			Build()).
		Build())
	registryManifest.DefineType("Entry", entryType)

	source := `
local registry = require("registry")

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
            local suite_name = tostring(suite)
            suites[suite_name] = suites[suite_name] or {}
            table.insert(suites[suite_name], entry)
        else
            table.insert(no_suite, entry)
        end
    end

    return suites, no_suite
end

local function run_suite(name: string, tests: {any})
    return #tests, {}
end

local function run_tests()
    local entries, err = registry.find({["meta.type"] = "test"})
    if err then
        return 1
    end
    if not entries or #entries == 0 then
        return 0
    end

    local suites, no_suite = group_by_suite(entries)
    local suite_names = sorted_keys(suites)

    for _, name in ipairs(suite_names) do
        local count = run_suite(name, suites[name])
    end
    return 0
end
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("registry", registryManifest))
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors for registry.find any-valued suite metadata with explicit string normalization")
	}
}

// TestLinterFalsePositive_WippyRunner_GroupBySuite reproduces the test runner
// regression where entries are not narrowed after err guards, causing any[]
// to leak into sort_tests(no_suite).
func TestLinterFalsePositive_WippyRunner_GroupBySuite(t *testing.T) {
	registryManifest := io.NewManifest("registry")
	entryType := typ.NewRecord().
		Field("data", typ.Any).
		Field("id", typ.String).
		Field("kind", typ.String).
		Field("meta", typ.NewMap(typ.String, typ.Any)).
		Build()
	registryManifest.SetExport(typ.NewRecord().
		Field("find", typ.Func().
			Param("query", typ.Any).
			Returns(typ.NewArray(entryType), typ.NewOptional(typ.LuaError)).
			Build()).
		Build())
	registryManifest.DefineType("Entry", entryType)

	source := `
local registry = require("registry")

local function sort_tests(tests)
    table.sort(tests, function(a, b)
        local order_a = (a.meta and a.meta.order) or 0
        local order_b = (b.meta and b.meta.order) or 0
        if order_a ~= order_b then
            return order_a < order_b
        end
        return a.id < b.id
    end)
    return tests
end

local function group_by_suite(entries)
    local suites: {[string]: any[]} = {}
    local no_suite: any[] = {}

    for _, entry in ipairs(entries) do
        local suite = entry.meta and entry.meta.suite
        if suite then
            suites[suite] = suites[suite] or {}
            table.insert(suites[suite], entry)
        else
            table.insert(no_suite, entry)
        end
    end

    for _, tests in pairs(suites) do
        sort_tests(tests)
    end
    sort_tests(no_suite)

    return suites, no_suite
end

local entries, err = registry.find({["meta.type"] = "test"})
if err then
    return
end
if not entries or #entries == 0 then
    return
end

local suites, no_suite = group_by_suite(entries)
local _ = suites
local _ = no_suite
`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("registry", registryManifest))
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors for wippy runner group_by_suite pattern")
	}
}

// TestLinterFalsePositive_UnionTupleMerge tests that union of tuples merges
// position-wise: (A, B) | (A, C) becomes (A, B | C) not a union of tuples.
// This is the core fix for the wippy test_runner arithmetic error.
func TestLinterFalsePositive_UnionTupleMerge(t *testing.T) {
	source := `
local function process(x: string | number)
    if type(x) == "string" then
        return 1, {x}
    else
        return 2, {tostring(x)}
    end
end

local count, results = process("test")
local total = 0 + count
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s", e.Message)
		}
		t.Fatal("expected no errors when extracting first value from multi-return")
	}
}

// TestLinterFalsePositive_UnionCallMultiReturn tests that calling a union of
// functions merges return tuples position-wise instead of creating union of tuples.
func TestLinterFalsePositive_UnionCallMultiReturn(t *testing.T) {
	source := `
type Handler = ((x: any) -> (integer, string[])) | ((x: any) -> (integer, number[]))

local function call_handler(h: Handler)
    local count, results = h("test")
    local total = 0 + count
    return total
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s", e.Message)
		}
		t.Fatal("expected no errors - union call should merge tuples position-wise")
	}
}

// TestLinterFalsePositive_MultiReturnFromUnionCall tests arithmetic on first
// return value when function is called with different argument types.
func TestLinterFalsePositive_MultiReturnFromUnionCall(t *testing.T) {
	source := `
local function run(tests)
    local failures = {}
    for _, t in ipairs(tests) do
        if t.fail then
            table.insert(failures, t)
        end
    end
    return #tests, failures
end

local a: {any}[] = {{}}
local b: {fail: boolean}[] = {{fail = true}}

local count1, f1 = run(a)
local count2, f2 = run(b)

local total = count1 + count2
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s", e.Message)
		}
		t.Fatal("expected no errors with multi-return arithmetic")
	}
}

// TestLinterFalsePositive_KeysProvenanceIterator tests that iterating over
// keys-provenance variables (result of sorted_keys) uses the original table's
// key type for the iterator variable.
func TestLinterFalsePositive_KeysProvenanceIterator(t *testing.T) {
	source := `
local function sorted_keys(tbl)
    local keys = {}
    for k in pairs(tbl) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

local suites: {[string]: {any}} = {}
suites["a"] = {1}
suites["b"] = {2}

local suite_names = sorted_keys(suites)

for idx, name in ipairs(suite_names) do
    local tests = suites[name]
    local present = tests ~= nil
end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors - keys provenance should derive type from original table")
	}
}

// TestLinterFalsePositive_WippyRunner_RunTestParamFlow reproduces the app runner
// pattern where run_suite iterates tests and forwards each entry into run_test,
// which calls funcs.call(entry.id) inside pcall.
func TestLinterFalsePositive_WippyRunner_RunTestParamFlow(t *testing.T) {
	registryManifest := io.NewManifest("registry")
	entryType := typ.NewRecord().
		Field("id", typ.String).
		Field("name", typ.String).
		Field("meta", typ.NewOptional(typ.NewMap(typ.String, typ.Any))).
		Build()
	registryManifest.SetExport(typ.NewRecord().
		Field("find", typ.Func().
			Param("query", typ.Any).
			Returns(typ.NewArray(entryType)).
			Build()).
		Build())
	registryManifest.DefineType("Entry", entryType)

	funcsManifest := io.NewManifest("funcs")
	funcsManifest.SetExport(typ.NewRecord().
		Field("call", typ.Func().
			Param("name", typ.String).
			Variadic(typ.Any).
			Returns(typ.Any, typ.NewOptional(typ.LuaError)).
			Build()).
		Build())

	source := `
local registry = require("registry")
local funcs = require("funcs")

local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

local function sort_tests(tests)
    table.sort(tests, function(a, b)
        local order_a = (a.meta and a.meta.order) or 0
        local order_b = (b.meta and b.meta.order) or 0
        if order_a ~= order_b then
            return order_a < order_b
        end
        return a.id < b.id
    end)
    return tests
end

local function group_by_suite(entries)
    local suites: {[string]: any[]} = {}
    local no_suite: any[] = {}

    for _, entry in ipairs(entries) do
        local suite = entry.meta and entry.meta.suite
        if suite then
            suites[suite] = suites[suite] or {}
            table.insert(suites[suite], entry)
        else
            table.insert(no_suite, entry)
        end
    end

    for _, tests in pairs(suites) do
        sort_tests(tests)
    end
    sort_tests(no_suite)

    return suites, no_suite
end

local function run_test(entry)
    local ok, result, err = pcall(function()
        return funcs.call(entry.id)
    end)
    if not ok then
        return false, result
    end
    if err then
        return false, err
    end
    return true, nil
end

local function run_suite(name: string, tests: {any})
    for _, entry in ipairs(tests) do
        local ok, err = run_test(entry)
    end
    return #tests
end

local entries = registry.find({["meta.type"] = "test"})
local suites, no_suite = group_by_suite(entries)
local suite_names = sorted_keys(suites)

for _, name in ipairs(suite_names) do
    local count = run_suite(name, suites[name])
end

if #no_suite > 0 then
    local count = run_suite("other", no_suite)
end
`

	result := testutil.Check(source, testutil.WithStdlib(),
		testutil.WithManifest("registry", registryManifest),
		testutil.WithManifest("funcs", funcsManifest))
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors in run_suite -> run_test entry flow")
	}
}
