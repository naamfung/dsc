package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Regression: call-site param hints must keep informative soft map shapes.
// If {[string]: any[]} is dropped as "soft", sorted key iteration degrades to
// `name: any`, which then breaks suites[name] and downstream run_test(entry.id).
func TestWippyRunner_SortedKeysRetainsMapKeyHints(t *testing.T) {
	entryType := typ.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewOptional(typ.NewRecord().
			Field("suite", typ.NewOptional(typ.String)).
			Build())).
		Build()

	registryManifest := io.NewManifest("registry")
	registryManifest.SetExport(typ.NewRecord().
		Field("find", typ.Func().
			Param("kind", typ.String).
			Returns(typ.NewArray(entryType)).
			Build()).
		Build())

	funcsManifest := io.NewManifest("funcs")
	funcsManifest.SetExport(typ.NewRecord().
		Field("call", typ.Func().
			Param("id", typ.String).
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

			return suites, no_suite
		end

		local function run_test(entry)
			return funcs.call(entry.id)
		end

		local function run_suite(name: string, tests: {any})
			for _, entry in ipairs(tests) do
				run_test(entry)
			end
			return #tests
		end

		local entries = registry.find("test")
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
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Fatal("expected no errors for sorted_keys/group_by_suite hint propagation")
	}
}

func TestWippyRunner_SortedKeysWithFilterBranch(t *testing.T) {
	entryType := typ.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewOptional(typ.NewRecord().
			Field("suite", typ.NewOptional(typ.String)).
			Build())).
		Build()

	registryManifest := io.NewManifest("registry")
	registryManifest.SetExport(typ.NewRecord().
		Field("find", typ.Func().
			Param("kind", typ.String).
			Returns(typ.NewArray(entryType)).
			Build()).
		Build())

	funcsManifest := io.NewManifest("funcs")
	funcsManifest.SetExport(typ.NewRecord().
		Field("call", typ.Func().
			Param("id", typ.String).
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

		local function filter_tests(entries, patterns)
			if not patterns or #patterns == 0 then
				return entries
			end
			local filtered = {}
			for _, entry in ipairs(entries) do
				for _, pattern in ipairs(patterns) do
					if entry.id:find(pattern, 1, true) then
						table.insert(filtered, entry)
						break
					end
				end
			end
			return filtered
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
			return suites, no_suite
		end

		local function run_test(entry)
			return funcs.call(entry.id)
		end

		local function run_suite(name: string, tests: {any})
			for _, entry in ipairs(tests) do
				run_test(entry)
			end
			return #tests
		end

		local io = require("io")
		local entries = registry.find("test")
		local args = io.args()
		if args and #args > 0 then
			entries = filter_tests(entries, args)
		end

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
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Fatal("expected no errors with filter branch")
	}
}

func TestWippyRunner_NearLiteralTestRunnerFlow(t *testing.T) {
	entryType := typ.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewOptional(typ.NewRecord().
			Field("suite", typ.NewOptional(typ.String)).
			Field("order", typ.NewOptional(typ.Number)).
			Build())).
		Build()

	registryManifest := io.NewManifest("registry")
	registryManifest.SetExport(typ.NewRecord().
		Field("find", typ.Func().
			Param("query", typ.Any).
			Returns(typ.NewArray(entryType), typ.NewOptional(typ.LuaError)).
			Build()).
		Build())

	funcsManifest := io.NewManifest("funcs")
	funcsManifest.SetExport(typ.NewRecord().
		Field("call", typ.Func().
			Param("id", typ.String).
			Variadic(typ.Any).
			Returns(typ.Any, typ.NewOptional(typ.LuaError)).
			Build()).
		Build())

	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(typ.NewRecord().
		Field("MILLISECOND", typ.Number).
		Field("sleep", typ.Func().Param("d", typ.Number).Returns().Build()).
		Build())

	source := `
		local io = require("io")
		local registry = require("registry")
		local funcs = require("funcs")
		local time = require("time")

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

		local function sorted_keys(t)
			local keys = {}
			for k in pairs(t) do
				table.insert(keys, k)
			end
			table.sort(keys)
			return keys
		end

		local function run_test(entry)
			local max_retries = 3
			local retry_delay = 10 * time.MILLISECOND
			for attempt = 1, max_retries do
				local ok, result, err = pcall(function()
					return funcs.call(entry.id)
				end)
				if not ok then
					local err_str = tostring(result)
					if err_str:match("pool not found") and attempt < max_retries then
						time.sleep(retry_delay)
					else
						return false, result
					end
				elseif err then
					return false, err
				elseif result == false then
					return false, "test returned false"
				else
					return true, nil
				end
			end
			return false, "max retries exceeded"
		end

		local function run_suite(name: string, tests: {any}, suite_idx: number, total_suites: number, completed_tests: number, total_tests: number)
			local failures = {}
			for _, entry in ipairs(tests) do
				local ok, err_obj = run_test(entry)
				if not ok then
					table.insert(failures, {
						id = entry.id,
						error = err_obj
					})
				end
			end
			return #tests, failures
		end

		local function filter_tests(entries, patterns)
			if not patterns or #patterns == 0 then
				return entries
			end
			local filtered = {}
			for _, entry in ipairs(entries) do
				for _, pattern in ipairs(patterns) do
					if entry.id:find(pattern, 1, true) then
						table.insert(filtered, entry)
						break
					end
				end
			end
			return filtered
		end

		local function run_tests()
			local entries, err = registry.find({["meta.type"] = "test"})
			if err then
				return 1
			end
			if not entries or #entries == 0 then
				return 0
			end

			local args = io.args()
			if args and #args > 0 then
				entries = filter_tests(entries, args)
				if #entries == 0 then
					return 0
				end
			end

			local suites, no_suite = group_by_suite(entries)
			local suite_names = sorted_keys(suites)
			local completed_tests = 0
			local total_tests = #entries
			local total_suites = #suite_names + (#no_suite > 0 and 1 or 0)
			for idx, name in ipairs(suite_names) do
				local count, failures = run_suite(name, suites[name], idx, total_suites, completed_tests, total_tests)
				completed_tests = completed_tests + count
			end
			if #no_suite > 0 then
				local count, failures = run_suite("other", no_suite, total_suites, total_suites, completed_tests, total_tests)
				completed_tests = completed_tests + count
			end
			return 0
		end

		run_tests()
	`

	result := testutil.Check(source, testutil.WithStdlib(),
		testutil.WithManifest("registry", registryManifest),
		testutil.WithManifest("funcs", funcsManifest),
		testutil.WithManifest("time", timeManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		if result.Session != nil && result.Session.Store != nil && result.Session.RootResult != nil {
			root := result.Session.RootResult.Graph
			parentHash := result.Session.Store.GraphParentHashOf(root.ID())
			parent := result.Session.Store.Parents()[parentHash]
			localTypes := result.Session.Store.GetLocalFuncTypesSnapshot(root, parent)
			hints := result.Session.Store.GetParamHintsSnapshot(root, parent)
			if bindings := result.Session.Store.ModuleBindings(); bindings != nil {
				for sym, fnType := range localTypes {
					name := bindings.Name(sym)
					if name == "sorted_keys" || name == "run_suite" || name == "run_test" || name == "group_by_suite" {
						t.Logf("local-fn %q sym=%d type=%s", name, sym, typ.Format(fnType, typ.DefaultFormatOptions))
						if hv := hints[sym]; len(hv) > 0 {
							t.Logf("param-hints %q: %v", name, hv)
						}
					}
				}
			}
			for _, fr := range result.Session.ResultsMap() {
				if fr == nil || fr.Graph == nil || fr.NarrowSynth == nil {
					continue
				}
				funcName := ""
				if fn := result.Session.Store.FuncForGraph(fr.Graph); fn != nil {
					if sym, ok := result.Session.Store.SymbolForFunc(fn); ok {
						if b := result.Session.Store.ModuleBindings(); b != nil {
							funcName = b.Name(sym)
						}
					}
				}
				if funcName == "" {
					funcName = "<anon>"
				}
				t.Logf("graph id=%d func=%q", fr.Graph.ID(), funcName)
				fr.Graph.EachCallSite(func(p cfg.Point, ci *cfg.CallInfo) {
					if ci == nil {
						return
					}
					calleeName := ci.CalleeName
					if calleeName == "" {
						if ident, ok := ci.Callee.(*ast.IdentExpr); ok {
							calleeName = ident.Value
						}
					}
					if funcName == "run_tests" {
						var a0, a1 string
						if len(ci.Args) > 0 {
							a0 = typ.FormatShort(fr.NarrowSynth.TypeOf(ci.Args[0], p))
						}
						if len(ci.Args) > 1 {
							a1 = typ.FormatShort(fr.NarrowSynth.TypeOf(ci.Args[1], p))
						}
						t.Logf("run_tests call callee=%q method=%q args=%d a0=%s a1=%s", calleeName, ci.Method, len(ci.Args), a0, a1)
					}
					if calleeName == "sorted_keys" && len(ci.Args) > 0 {
						argType := fr.NarrowSynth.TypeOf(ci.Args[0], p)
						t.Logf("graph=%q call sorted_keys arg0 type=%s kind=%v", funcName, typ.Format(argType, typ.FormatOptions{
							MaxDepth:        20,
							MaxNodes:        2000,
							MaxUnionMembers: 20,
							MaxRecordFields: 50,
							MaxTupleElems:   20,
							MaxTypeParams:   20,
							MaxParams:       20,
							MaxReturns:      20,
							MaxBytes:        4000,
						}), argType.Kind())
						if rec, ok := argType.(*typ.Record); ok {
							t.Logf("graph=%q call sorted_keys arg0 record fields=%d open=%v map=%v mapKey=%s mapVal=%s", funcName, len(rec.Fields), rec.Open, rec.HasMapComponent(), typ.FormatShort(rec.MapKey), typ.FormatShort(rec.MapValue))
						}
					}
					if calleeName == "run_suite" && len(ci.Args) > 1 {
						argType := fr.NarrowSynth.TypeOf(ci.Args[1], p)
						t.Logf("graph=%q call run_suite arg1 type=%s kind=%v", funcName, typ.Format(argType, typ.FormatOptions{
							MaxDepth:        20,
							MaxNodes:        2000,
							MaxUnionMembers: 20,
							MaxRecordFields: 50,
							MaxTupleElems:   20,
							MaxTypeParams:   20,
							MaxParams:       20,
							MaxReturns:      20,
							MaxBytes:        4000,
						}), argType.Kind())
						if rec, ok := argType.(*typ.Record); ok {
							t.Logf("graph=%q call run_suite arg1 record fields=%d open=%v map=%v mapKey=%s mapVal=%s", funcName, len(rec.Fields), rec.Open, rec.HasMapComponent(), typ.FormatShort(rec.MapKey), typ.FormatShort(rec.MapValue))
						}
					}
				})
			}
		}
		t.Fatal("expected no errors for near-literal app:test_runner flow")
	}
}
