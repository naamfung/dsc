package modules

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestManifest_BasicExport tests basic manifest export types.
func TestManifest_BasicExport(t *testing.T) {
	// Create a simple math module
	mathManifest := io.NewManifest("mymath")
	mathType := typ.NewRecord().
		Field("add", typ.Func().Param("a", typ.Number).Param("b", typ.Number).Returns(typ.Number).Build()).
		Field("PI", typ.Number).
		Build()
	mathManifest.SetExport(mathType)

	source := `
		local m = require("mymath")
		local sum: number = m.add(1, 2)
		local pi: number = m.PI
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("mymath", mathManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with manifest export")
	}
}

// TestManifest_GenericExport tests manifest with generic types.
func TestManifest_GenericExport(t *testing.T) {
	// Create a container module with generic types
	containerManifest := io.NewManifest("container")

	elemType := typ.NewTypeParam("T", nil)
	boxType := typ.NewRecord().Field("value", elemType).Build()
	boxGeneric := typ.NewGeneric("Box", []*typ.TypeParam{elemType}, boxType)

	containerManifest.DefineType("Box", boxGeneric)

	newBoxFn := typ.Func().Param("value", typ.Any).Returns(typ.Any).Build()
	exportType := typ.NewRecord().Field("new_box", newBoxFn).Build()
	containerManifest.SetExport(exportType)

	source := `
		local c = require("container")
		local b = c.new_box(42)
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("container", containerManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with generic manifest")
	}
}

// TestManifest_LocalRequireInFunction enforces manifest-based typing for local require aliases.
func TestManifest_LocalRequireInFunction(t *testing.T) {
	mathManifest := io.NewManifest("mymath")
	mathType := typ.NewRecord().
		Field("add", typ.Func().Param("a", typ.Number).Param("b", typ.Number).Returns(typ.Number).Build()).
		Build()
	mathManifest.SetExport(mathType)

	source := `
		local function f()
			local m = require("mymath")
			return m.add("bad", 2)
		end
		local x = f()
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("mymath", mathManifest))
	if !result.HasError() {
		t.Fatalf("expected error for wrong arg type when using local require alias")
	}
}

// TestManifest_SoftAnnotationParamHints ensures soft annotations like {any}
// are overridden by call-site param hints.
func TestManifest_SoftAnnotationParamHints(t *testing.T) {
	registryManifest := io.NewManifest("registry")
	entryType := typ.NewRecord().Field("id", typ.String).Build()
	findFn := typ.Func().Param("query", typ.Any).Returns(typ.NewArray(entryType)).Build()
	registryManifest.SetExport(typ.NewRecord().Field("find", findFn).Build())

	source := `
		local registry = require("registry")

		local funcs = {}
		function funcs.call(id: string) end

		local function run_suite(name: string, tests: {any})
			for _, entry in ipairs(tests) do
				local ok = pcall(function()
					return funcs.call(entry.id)
				end)
			end
		end

		run_suite("main", registry.find({}))
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("registry", registryManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with soft annotation param hints")
	}
}

// TestManifest_SoftLocalAnnotations ensures soft local annotations are refined
// by table mutations and indexer assignments in return inference.
func TestManifest_SoftLocalAnnotations(t *testing.T) {
	registryManifest := io.NewManifest("registry")
	metaType := typ.NewMap(typ.String, typ.Any)
	entryType := typ.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewOptional(metaType)).
		Build()
	findFn := typ.Func().
		Param("query", typ.Any).
		Returns(typ.NewArray(entryType), typ.NewOptional(typ.LuaError)).
		Build()
	registryManifest.SetExport(typ.NewRecord().Field("find", findFn).Build())

	source := `
		local registry = require("registry")
		local funcs = require("funcs")

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

		local function run_suite(name: string, tests: {any})
			for _, entry in ipairs(tests) do
				funcs.call(entry.id)
			end
		end

		local entries, err = registry.find({["meta.type"] = "test"})
		if not err then
			local suites, no_suite = group_by_suite(entries)
			for name, tests in pairs(suites) do
				run_suite(name, tests)
			end
			run_suite("no-suite", no_suite)
		end
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("registry", registryManifest),
		testutil.WithManifest("funcs", testutil.FuncsManifest()),
	)
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with soft local annotations")
	}
}

// TestManifest_InterfaceExport tests manifest with interface types.
func TestManifest_InterfaceExport(t *testing.T) {
	// Create a logger module with interface
	loggerManifest := io.NewManifest("logger")

	loggerInterface := typ.NewInterface("Logger", []typ.Method{
		{Name: "info", Type: typ.Func().Param("self", typ.Self).Param("msg", typ.String).Build()},
		{Name: "error", Type: typ.Func().Param("self", typ.Self).Param("msg", typ.String).Build()},
	})
	loggerManifest.DefineType("Logger", loggerInterface)

	exportType := typ.NewRecord().
		Field("new", typ.Func().Returns(loggerInterface).Build()).
		Build()
	loggerManifest.SetExport(exportType)

	source := `
		local l = require("logger")
		local log = l.new()
		log:info("hello")
		log:error("oops")
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("logger", loggerManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with interface manifest")
	}
}

// TestManifest_SummaryNarrowing tests manifest with summary-based narrowing.
func TestManifest_SummaryNarrowing(t *testing.T) {
	// Create validator module with summary for narrowing
	validatorManifest := io.NewManifest("validator")

	// Bare function with no inline spec
	notNilFn := typ.Func().Param("x", typ.Any).Returns(typ.Any).Build()
	exportType := typ.NewRecord().Field("not_nil", notNilFn).Build()
	validatorManifest.SetExport(exportType)

	// Add summary with NotNil constraint
	summary := io.NewSummary([]typ.Type{typ.Any}, []typ.Type{typ.Any})
	summary.Ensures = constraint.FromConstraints(constraint.NotNil{
		Path: constraint.Path{Root: "$0"},
	})
	validatorManifest.DefineSummary("not_nil", summary)

	source := `
		local v = require("validator")

		local function get_value(): string?
			return nil
		end

		local function main()
			local x = get_value()
			v.not_nil(x)
			local len = #x
			return len
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("validator", validatorManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with summary narrowing")
	}
}

// TestManifest_CrossModuleTypes tests types shared across modules.
func TestManifest_CrossModuleTypes(t *testing.T) {
	// Create two modules that share a type
	typesManifest := io.NewManifest("types")
	personType := typ.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Number).
		Build()
	typesManifest.DefineType("Person", personType)
	typesManifest.SetExport(typ.NewRecord().Build())

	usersManifest := io.NewManifest("users")
	usersExport := typ.NewRecord().
		Field("create", typ.Func().Param("name", typ.String).Param("age", typ.Number).Returns(personType).Build()).
		Build()
	usersManifest.SetExport(usersExport)

	source := `
		local users = require("users")
		local p = users.create("Alice", 30)
		local name: string = p.name
		local age: number = p.age
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("types", typesManifest),
		testutil.WithManifest("users", usersManifest),
	)
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with cross-module types")
	}
}

// TestManifest_WrongTypeFromModule tests that wrong types from modules are caught.
func TestManifest_WrongTypeFromModule(t *testing.T) {
	mathManifest := io.NewManifest("mymath")
	mathType := typ.NewRecord().
		Field("add", typ.Func().Param("a", typ.Number).Param("b", typ.Number).Returns(typ.Number).Build()).
		Build()
	mathManifest.SetExport(mathType)

	source := `
		local m = require("mymath")
		local sum: string = m.add(1, 2)
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("mymath", mathManifest))
	if !result.HasError() {
		t.Errorf("expected error assigning number to string")
	}
}

// TestManifest_AssertNotNil_Serialization tests that narrowing works through
// manifest serialization (encode/decode cycle).
func TestManifest_AssertNotNil_Serialization(t *testing.T) {
	// Create assert module that narrows via return value
	assertModule := testutil.CheckAndExport(`
		local M = {}
		function M.not_nil(x)
			if x == nil then error("nil") end
			return x
		end
		return M
	`, "assert", testutil.WithStdlib())

	if assertModule.HasError() {
		t.Fatalf("assert module should have no errors: %v", assertModule.Errors)
	}

	// Encode and decode to verify binary serialization works
	encoded, err := assertModule.Manifest.Encode()
	if err != nil {
		t.Fatalf("manifest encode error: %v", err)
	}

	decodedManifest, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("manifest decode error: %v", err)
	}

	source := `
		local assert = require("assert")

		local function get_value(): string?
			return nil
		end

		local function main()
			local x = get_value()
			assert.not_nil(x)
			local len = #x
			return len
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("assert", decodedManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with serialized narrowing manifest")
	}
}

// TestManifest_AssertNotNil_FieldAccess tests narrowing allows field access after assert.
func TestManifest_AssertNotNil_FieldAccess(t *testing.T) {
	assertModule := testutil.CheckAndExport(`
		local M = {}
		function M.not_nil(x)
			if x == nil then error("nil") end
			return x
		end
		return M
	`, "assert", testutil.WithStdlib())

	if assertModule.HasError() {
		t.Fatalf("assert module should have no errors: %v", assertModule.Errors)
	}

	source := `
		local assert = require("assert")

		type Node = {kind: string}

		local function get_node(): Node?
			return nil
		end

		local function main()
			local node = get_node()
			assert.not_nil(node)
			local k = node.kind
			return k
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("assert", assertModule))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with field access after narrowing")
	}
}

// TestManifest_AssertNotNil_Assignment tests narrowing allows assignment to non-optional.
func TestManifest_AssertNotNil_Assignment(t *testing.T) {
	assertModule := testutil.CheckAndExport(`
		local M = {}
		function M.not_nil(x)
			if x == nil then error("nil") end
			return x
		end
		return M
	`, "assert", testutil.WithStdlib())

	if assertModule.HasError() {
		t.Fatalf("assert module should have no errors: %v", assertModule.Errors)
	}

	source := `
		local assert = require("assert")

		local function get_number(): number?
			return nil
		end

		local function main()
			local x = get_number()
			assert.not_nil(x)
			local y: number = x
			return y
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("assert", assertModule))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with assignment after narrowing")
	}
}

// TestManifest_ScopeShadowingWithRequire tests that require shadows base scope symbols.
func TestManifest_ScopeShadowingWithRequire(t *testing.T) {
	// Create http module (server-side, takes 0-1 args)
	httpManifest := io.NewManifest("http")
	httpType := typ.NewInterface("http", []typ.Method{
		{Name: "request", Type: typ.Func().OptParam("config", typ.Any).Returns(typ.Any).Build()},
	})
	httpManifest.SetExport(httpType)

	// Create http_client module (takes 2-3 args)
	httpClientManifest := io.NewManifest("http_client")
	httpClientType := typ.NewInterface("http_client", []typ.Method{
		{Name: "request", Type: typ.Func().Param("method", typ.String).Param("url", typ.String).OptParam("opts", typ.Any).Returns(typ.Any).Build()},
	})
	httpClientManifest.SetExport(httpClientType)

	source := `
		local http = require("http_client")
		local resp = http.request("OPTIONS", "http://localhost/test", {})
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("http", httpManifest),
		testutil.WithManifest("http_client", httpClientManifest),
	)
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors - require should shadow base scope")
	}
}

// TestManifest_VariableNameDoesNotMatchModulePath tests that manifest enrichment
// uses the actual required module, not a module that happens to match the variable name.
// Bug: When both "http" and "http_client" modules exist, and code does:
//
//	local http = require("http_client")
//	local resp = http.request(...)
//
// The manifest enrichment was incorrectly using "http" module (matching variable name)
// instead of "http_client" module (the actual required module).
func TestManifest_VariableNameDoesNotMatchModulePath(t *testing.T) {
	// Create http module (server-side) - returns http.Request type
	httpManifest := io.NewManifest("http")
	httpRequestType := typ.NewRecord().
		Field("method", typ.String).
		Field("path", typ.String).
		Build()
	httpManifest.DefineType("Request", httpRequestType)
	httpType := typ.NewRecord().
		Field("request", typ.Func().Returns(httpRequestType).Build()).
		Build()
	httpManifest.SetExport(httpType)

	// Create http_client module - returns http_client.Response type with different fields
	httpClientManifest := io.NewManifest("http_client")
	httpClientResponseType := typ.NewRecord().
		Field("status_code", typ.Number).
		Field("body", typ.String).
		Build()
	httpClientManifest.DefineType("Response", httpClientResponseType)
	httpClientType := typ.NewRecord().
		Field("request", typ.Func().Param("method", typ.String).Param("url", typ.String).Returns(httpClientResponseType).Build()).
		Build()
	httpClientManifest.SetExport(httpClientType)

	// This code assigns the result to a variable expecting http_client.Response fields
	// If the bug exists, it will try to use http.Request type which lacks status_code
	source := `
		local http = require("http_client")
		local resp = http.request("GET", "http://example.com")
		local status: number = resp.status_code
		local body: string = resp.body
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("http", httpManifest),
		testutil.WithManifest("http_client", httpClientManifest),
	)
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors - variable 'http' should use http_client module type, not http module type")
	}
}
