package modules

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// =============================================================================
// End-to-End Tests (using CheckAndExport)
// =============================================================================

// TestE2E_SimpleModuleExport tests end-to-end: check module -> export -> require
func TestE2E_SimpleModuleExport(t *testing.T) {
	// Check and export provider module
	mathMod := testutil.CheckAndExport(`
		local M = {}

		function M.add(a: number, b: number): number
			return a + b
		end

		function M.multiply(a: number, b: number): number
			return a * b
		end

		return M
	`, "mymath", testutil.WithStdlib())

	if mathMod.HasError() {
		for _, e := range mathMod.Errors {
			t.Logf("provider error: %s", e.Message)
		}
		t.Fatal("provider module has errors")
	}

	// Consumer requires with alias
	result := testutil.Check(`
		local math = require("mymath")
		local sum: number = math.add(1, 2)
		local product: number = math.multiply(3, 4)
	`, testutil.WithStdlib(), testutil.WithModule("mymath", mathMod))

	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("consumer error: %s", e.Message)
		}
		t.Error("expected no errors with aliased require")
	}
}

// TestE2E_ModuleLiteralTableFieldsPreserved ensures literal field types
// survive export/require without degrading to any/unknown.
func TestE2E_ModuleLiteralTableFieldsPreserved(t *testing.T) {
	constsMod := testutil.CheckAndExport(`
		return {
			APP_DB = "postgres://localhost/myapp",
			CACHE_TTL = 300,
			DEBUG = true
		}
	`, "consts", testutil.WithStdlib())

	if constsMod.HasError() {
		for _, e := range constsMod.Errors {
			t.Logf("provider error: %s", e.Message)
		}
		t.Fatal("provider module has errors")
	}

	result := testutil.Check(`
		local consts = require("consts")
		local dsn: string = consts.APP_DB
		local ttl: number = consts.CACHE_TTL
		local dbg: boolean = consts.DEBUG
	`, testutil.WithStdlib(), testutil.WithModule("consts", constsMod))

	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("consumer error: %s", e.Message)
		}
		t.Error("expected no errors when reading literal table fields from manifest export")
	}
}

// TestE2E_TypeDefinitionExport tests module-local type export
func TestE2E_TypeDefinitionExport(t *testing.T) {
	// Provider defines a type
	geoMod := testutil.CheckAndExport(`
		type Point = {x: number, y: number}

		local M = {}

		function M.origin(): Point
			return {x = 0, y = 0}
		end

		function M.create(x: number, y: number): Point
			return {x = x, y = y}
		end

		return M
	`, "geometry", testutil.WithStdlib())

	if geoMod.HasError() {
		for _, e := range geoMod.Errors {
			t.Logf("provider error: %s", e.Message)
		}
		t.Fatal("provider has errors")
	}

	// Consumer uses the exported type
	result := testutil.Check(`
		local geo = require("geometry")
		local p = geo.create(3, 4)
		local x: number = p.x
		local y: number = p.y
	`, testutil.WithStdlib(), testutil.WithModule("geometry", geoMod))

	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s", e.Message)
		}
		t.Error("expected no errors")
	}
}

// TestE2E_TypeOfDefinitionExport ensures typeof(...) captures annotated local shapes.
func TestE2E_TypeOfDefinitionExport(t *testing.T) {
	mod := testutil.CheckAndExport(`
		local sample: {name: string, age: number} = {name = "Ada", age = 33}
		type Sample = typeof(sample)

		local M = {}
		function M.accept(s: Sample): Sample
			return s
		end
		return M
	`, "sample", testutil.WithStdlib())

	if mod.HasError() {
		for _, e := range mod.Errors {
			t.Logf("provider error: %s", e.Message)
		}
		t.Fatal("provider has errors")
	}

	sampleType := mod.Manifest.Types["Sample"]
	if sampleType == nil {
		t.Fatal("expected Sample type in manifest")
	}

	rec, ok := unwrap.Alias(sampleType).(*typ.Record)
	if !ok {
		t.Fatalf("expected Sample to resolve to record, got %T", sampleType)
	}

	nameField := rec.GetField("name")
	if nameField == nil {
		t.Fatal("expected Sample.name field")
	}
	if !typ.TypeEquals(nameField.Type, typ.String) {
		t.Errorf("expected Sample.name to be string, got %v", nameField.Type)
	}

	ageField := rec.GetField("age")
	if ageField == nil {
		t.Fatal("expected Sample.age field")
	}
	if !typ.TypeEquals(ageField.Type, typ.Number) {
		t.Errorf("expected Sample.age to be number, got %v", ageField.Type)
	}
}

// TestE2E_SerializationRoundTrip tests manifest encode/decode with real export
func TestE2E_SerializationRoundTrip(t *testing.T) {
	// Check and export
	mod := testutil.CheckAndExport(`
		local M = {}
		function M.greet(name: string): string
			return "Hello, " .. name
		end
		return M
	`, "greeter", testutil.WithStdlib())

	if mod.HasError() {
		for _, e := range mod.Errors {
			t.Logf("provider error: %s", e.Message)
		}
		t.Fatal("provider has errors")
	}

	// Serialize and deserialize
	encoded, err := mod.Manifest.Encode()
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	// Use decoded manifest
	result := testutil.Check(`
		local g = require("greeter")
		local msg: string = g.greet("World")
	`, testutil.WithStdlib(), testutil.WithManifest("greeter", decoded))

	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s", e.Message)
		}
		t.Error("expected no errors after serialization round-trip")
	}
}

// TestE2E_ChainedModules tests module A -> module B -> consumer
func TestE2E_ChainedModules(t *testing.T) {
	// Module A: defines base types and functions
	baseMod := testutil.CheckAndExport(`
		type User = {id: number, name: string}

		local M = {}
		function M.create(id: number, name: string): User
			return {id = id, name = name}
		end
		return M
	`, "base", testutil.WithStdlib())

	if baseMod.HasError() {
		for _, e := range baseMod.Errors {
			t.Logf("base error: %s", e.Message)
		}
		t.Fatal("base module has errors")
	}

	// Module B: uses A
	serviceMod := testutil.CheckAndExport(`
		local base = require("base")

		local M = {}
		function M.greet_user(u): string
			return "Hello, " .. u.name
		end
		return M
	`, "service", testutil.WithStdlib(), testutil.WithModule("base", baseMod))

	if serviceMod.HasError() {
		for _, e := range serviceMod.Errors {
			t.Logf("service error: %s", e.Message)
		}
		t.Fatal("service module has errors")
	}

	// Consumer uses both A and B
	result := testutil.Check(`
		local base = require("base")
		local service = require("service")

		local u = base.create(1, "Alice")
		local msg: string = service.greet_user(u)
	`, testutil.WithStdlib(), testutil.WithModule("base", baseMod), testutil.WithModule("service", serviceMod))

	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s", e.Message)
		}
		t.Error("expected no errors in chained modules")
	}
}

// TestE2E_WrongType tests that type errors are caught with exported modules
func TestE2E_WrongType(t *testing.T) {
	mod := testutil.CheckAndExport(`
		local M = {}
		function M.get_number(): number
			return 42
		end
		return M
	`, "nums", testutil.WithStdlib())

	if mod.HasError() {
		for _, e := range mod.Errors {
			t.Logf("provider error: %s", e.Message)
		}
		t.Fatal("provider has errors")
	}

	// Consumer assigns number to string - should error
	result := testutil.Check(`
		local nums = require("nums")
		local s: string = nums.get_number()
	`, testutil.WithStdlib(), testutil.WithModule("nums", mod))

	if !result.HasError() {
		t.Error("expected error assigning number to string")
	}
}

// TestE2E_MultipleExports tests multiple modules exported and consumed
func TestE2E_MultipleExports(t *testing.T) {
	// Module 1: json
	jsonMod := testutil.CheckAndExport(`
		local M = {}
		function M.encode(v: any): string
			return tostring(v)
		end
		function M.decode(s: string): any
			return s
		end
		return M
	`, "json", testutil.WithStdlib())

	// Module 2: http
	httpMod := testutil.CheckAndExport(`
		local M = {}
		function M.get(url: string): string
			return url
		end
		function M.post(url: string, body: string): string
			return body
		end
		return M
	`, "http", testutil.WithStdlib())

	// Consumer uses both
	result := testutil.Check(`
		local json = require("json")
		local http = require("http")

		local data = {name = "test"}
		local body: string = json.encode(data)
		local resp: string = http.post("https://api.example.com", body)
		local parsed = json.decode(resp)
	`, testutil.WithStdlib(), testutil.WithModule("json", jsonMod), testutil.WithModule("http", httpMod))

	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s", e.Message)
		}
		t.Error("expected no errors with multiple modules")
	}
}

// =============================================================================
// Manifest-Based Tests (using manually constructed manifests)
// =============================================================================

// TestModuleWorkflow_SimpleExport tests checking a module using its manifest.
func TestModuleWorkflow_SimpleExport(t *testing.T) {
	// Create manifest for math module (simulates export from type-checked module)
	mathManifest := io.NewManifest("mymath")
	mathType := typ.NewRecord().
		Field("add", typ.Func().Param("a", typ.Number).Param("b", typ.Number).Returns(typ.Number).Build()).
		Field("multiply", typ.Func().Param("a", typ.Number).Param("b", typ.Number).Returns(typ.Number).Build()).
		Build()
	mathManifest.SetExport(mathType)

	// Consumer module uses mymath
	consumerSource := `
		local math = require("mymath")
		local sum: number = math.add(1, 2)
		local product: number = math.multiply(3, 4)
	`

	result := testutil.Check(consumerSource, testutil.WithStdlib(), testutil.WithManifest("mymath", mathManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors using math module")
	}
}

// TestModuleWorkflow_AliasedRequire tests requiring a module with different local name.
func TestModuleWorkflow_AliasedRequire(t *testing.T) {
	// Create manifest for http_client module
	httpManifest := io.NewManifest("http_client")
	httpType := typ.NewRecord().
		Field("get", typ.Func().Param("url", typ.String).Returns(typ.String).Build()).
		Field("post", typ.Func().Param("url", typ.String).Param("body", typ.String).Returns(typ.String).Build()).
		Build()
	httpManifest.SetExport(httpType)

	// Consumer uses different local name (alias)
	consumerSource := `
		local http = require("http_client")
		local resp: string = http.get("https://example.com")
		local resp2: string = http.post("https://example.com", "data")
	`

	result := testutil.Check(consumerSource, testutil.WithStdlib(), testutil.WithManifest("http_client", httpManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with aliased require")
	}
}

// TestModuleWorkflow_ChainedModules tests module A -> module B -> consumer.
func TestModuleWorkflow_ChainedModules(t *testing.T) {
	// Module 1: types
	pointType := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()
	typesManifest := io.NewManifest("types")
	typesManifest.SetExport(typ.NewRecord().
		Field("create_point", typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(pointType).Build()).
		Build())
	typesManifest.DefineType("Point", pointType)

	// Module 2: geometry (uses types.Point)
	geometryManifest := io.NewManifest("geometry")
	geometryManifest.SetExport(typ.NewRecord().
		Field("distance", typ.Func().Param("p1", pointType).Param("p2", pointType).Returns(typ.Number).Build()).
		Field("origin", typ.Func().Returns(pointType).Build()).
		Build())

	// Consumer uses both
	consumerSource := `
		local types = require("types")
		local geo = require("geometry")

		local p1 = types.create_point(0, 0)
		local p2 = types.create_point(3, 4)
		local d: number = geo.distance(p1, p2)
		local origin = geo.origin()
		local x: number = origin.x
	`

	result := testutil.Check(consumerSource,
		testutil.WithStdlib(),
		testutil.WithManifest("types", typesManifest),
		testutil.WithManifest("geometry", geometryManifest),
	)
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with chained modules")
	}
}

// TestModuleWorkflow_SummaryNarrowing tests assert narrowing via manifest summary.
func TestModuleWorkflow_SummaryNarrowing(t *testing.T) {
	// Create assert module with summary for narrowing
	assertManifest := io.NewManifest("assert")
	notNilFn := typ.Func().Param("x", typ.Any).Returns(typ.Any).Build()
	assertManifest.SetExport(typ.NewRecord().Field("not_nil", notNilFn).Build())

	// Add summary with NotNil constraint
	summary := io.NewSummary([]typ.Type{typ.Any}, []typ.Type{typ.Any})
	summary.Ensures = constraint.FromConstraints(constraint.NotNil{
		Path: constraint.Path{Root: "$0"},
	})
	assertManifest.DefineSummary("not_nil", summary)

	// Consumer uses assert for narrowing
	consumerSource := `
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

	result := testutil.Check(consumerSource, testutil.WithStdlib(), testutil.WithManifest("assert", assertManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with summary narrowing")
	}
}

// TestModuleWorkflow_ExportedEffectSummaries verifies that OnReturn effects
// exported from a module are preserved in manifests and enable narrowing.
func TestModuleWorkflow_ExportedEffectSummaries(t *testing.T) {
	assertModule := testutil.CheckAndExport(`
		local assert = {}
		function assert.not_nil(v)
			if v == nil then error("nil") end
			return v
		end
		return assert
	`, "assert2", testutil.WithStdlib())

	if assertModule.HasError() {
		for _, d := range assertModule.Errors {
			t.Logf("assert2 error: %s", d.Message)
		}
		t.Fatalf("assert2 module should have no errors")
	}

	consumerSource := `
		local assert = require("assert2")

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

	result := testutil.Check(consumerSource,
		testutil.WithStdlib(),
		testutil.WithModule("assert2", assertModule),
	)
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with exported effect summaries")
	}
}

// TestModuleWorkflow_ExportTypeIncludesEffects verifies ExportType embeds function refinements
// for module functions (without relying on summaries).
func TestModuleWorkflow_ExportTypeIncludesEffects(t *testing.T) {
	result := testutil.Check(`
		local assert = {}
		function assert.not_nil(v)
			if v == nil then error("nil") end
			return v
		end
		return assert
	`, testutil.WithStdlib())

	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("module error: %s", d.Message)
		}
		t.Fatalf("module should have no errors")
	}

	export := result.Session.ExportType()
	rec, ok := export.(*typ.Record)
	if !ok {
		t.Fatalf("expected record export, got %T", export)
	}

	var fn *typ.Function
	for _, f := range rec.Fields {
		if f.Name == "not_nil" {
			fn, _ = f.Type.(*typ.Function)
			break
		}
	}
	if fn == nil {
		t.Fatalf("expected not_nil function in export")
	}
	if fn.Refinement == nil {
		t.Fatalf("expected not_nil to carry refinement in export")
	}
	if _, ok := fn.Refinement.(*constraint.FunctionRefinement); !ok {
		t.Fatalf("expected FunctionRefinement refinement, got %T", fn.Refinement)
	}
}

// TestModuleWorkflow_InterfaceExport tests interface method access through manifest.
func TestModuleWorkflow_InterfaceExport(t *testing.T) {
	// Create logger module with interface
	loggerManifest := io.NewManifest("logger")
	loggerInterface := typ.NewInterface("Logger", []typ.Method{
		{Name: "info", Type: typ.Func().Param("self", typ.Self).Param("msg", typ.String).Build()},
		{Name: "error", Type: typ.Func().Param("self", typ.Self).Param("msg", typ.String).Build()},
	})
	loggerManifest.DefineType("Logger", loggerInterface)
	loggerManifest.SetExport(typ.NewRecord().
		Field("new", typ.Func().Returns(loggerInterface).Build()).
		Build())

	consumerSource := `
		local logger = require("logger")
		local log = logger.new()
		log:info("hello")
		log:error("oops")
	`

	result := testutil.Check(consumerSource, testutil.WithStdlib(), testutil.WithManifest("logger", loggerManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with interface export")
	}
}

// TestModuleWorkflow_WrongTypeFromModule tests that type errors are caught.
func TestModuleWorkflow_WrongTypeFromModule(t *testing.T) {
	mathManifest := io.NewManifest("mymath")
	mathManifest.SetExport(typ.NewRecord().
		Field("add", typ.Func().Param("a", typ.Number).Param("b", typ.Number).Returns(typ.Number).Build()).
		Build())

	// Consumer assigns wrong type
	consumerSource := `
		local math = require("mymath")
		local sum: string = math.add(1, 2)
	`

	result := testutil.Check(consumerSource, testutil.WithStdlib(), testutil.WithManifest("mymath", mathManifest))
	if !result.HasError() {
		t.Errorf("expected error assigning number to string")
	}
}

// TestModuleWorkflow_ManifestSerialization tests manifest encode/decode cycle.
func TestModuleWorkflow_ManifestSerialization(t *testing.T) {
	originalManifest := io.NewManifest("mymath")
	originalManifest.SetExport(typ.NewRecord().
		Field("add", typ.Func().Param("a", typ.Number).Param("b", typ.Number).Returns(typ.Number).Build()).
		Build())

	// Encode and decode
	encoded, err := originalManifest.Encode()
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	decodedManifest, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	// Use decoded manifest
	consumerSource := `
		local math = require("mymath")
		local sum: number = math.add(1, 2)
	`

	result := testutil.Check(consumerSource, testutil.WithStdlib(), testutil.WithManifest("mymath", decodedManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors after manifest serialization")
	}
}

// TestModuleWorkflow_MultipleManifests tests using multiple manifests together.
func TestModuleWorkflow_MultipleManifests(t *testing.T) {
	// Module 1: json
	jsonManifest := io.NewManifest("json")
	jsonManifest.SetExport(typ.NewRecord().
		Field("encode", typ.Func().Param("v", typ.Any).Returns(typ.String).Build()).
		Field("decode", typ.Func().Param("s", typ.String).Returns(typ.Any).Build()).
		Build())

	// Module 2: http
	httpManifest := io.NewManifest("http")
	httpManifest.SetExport(typ.NewRecord().
		Field("get", typ.Func().Param("url", typ.String).Returns(typ.String).Build()).
		Field("post", typ.Func().Param("url", typ.String).Param("body", typ.String).Returns(typ.String).Build()).
		Build())

	// Consumer uses both
	consumerSource := `
		local json = require("json")
		local http = require("http")

		local data = {name = "test"}
		local body: string = json.encode(data)
		local resp: string = http.post("https://api.example.com", body)
		local result = json.decode(resp)
	`

	result := testutil.Check(consumerSource,
		testutil.WithStdlib(),
		testutil.WithManifest("json", jsonManifest),
		testutil.WithManifest("http", httpManifest),
	)
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with multiple manifests")
	}
}

// TestModuleWorkflow_NestedFunctionModuleAlias tests that module aliases from chunk level
// are visible inside nested functions.
func TestModuleWorkflow_NestedFunctionModuleAlias(t *testing.T) {
	// Create manifest for http_client module
	httpManifest := io.NewManifest("http_client")
	httpType := typ.NewRecord().
		Field("get", typ.Func().Param("url", typ.String).Returns(typ.String).Build()).
		Field("post", typ.Func().Param("url", typ.String).Param("body", typ.String).Returns(typ.String).Build()).
		Build()
	httpManifest.SetExport(httpType)

	// Module alias is declared at chunk level, used inside nested function
	consumerSource := `
		local http = require("http_client")

		local function fetch_data()
			return http.get("https://example.com")
		end

		local function send_data(data: string)
			return http.post("https://example.com", data)
		end

		local result: string = fetch_data()
	`

	result := testutil.Check(consumerSource, testutil.WithStdlib(), testutil.WithManifest("http_client", httpManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with nested function accessing top-level module alias")
	}
}

// TestModuleWorkflow_DeeplyNestedModuleAlias tests module alias access in deeply nested functions.
func TestModuleWorkflow_DeeplyNestedModuleAlias(t *testing.T) {
	// Create manifest
	utilManifest := io.NewManifest("util")
	utilType := typ.NewRecord().
		Field("format", typ.Func().Param("s", typ.String).Returns(typ.String).Build()).
		Build()
	utilManifest.SetExport(utilType)

	// Module alias used in nested function inside nested function
	consumerSource := `
		local util = require("util")

		local function outer()
			local function inner()
				return util.format("test")
			end
			return inner()
		end

		local result: string = outer()
	`

	result := testutil.Check(consumerSource, testutil.WithStdlib(), testutil.WithManifest("util", utilManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s (pos=%d:%d span=%d:%d-%d:%d)",
				d.Message,
				d.Position.Line, d.Position.Column,
				d.Span.StartLine, d.Span.StartCol, d.Span.EndLine, d.Span.EndCol,
			)
		}
		t.Errorf("expected no errors with deeply nested function accessing top-level module alias")
	}
}

// TestModuleWorkflow_GenericManifest tests generic types through manifest.
func TestModuleWorkflow_GenericManifest(t *testing.T) {
	// Create manifest with generic types
	containerManifest := io.NewManifest("container")

	elemType := typ.NewTypeParam("T", nil)
	boxType := typ.NewRecord().Field("value", elemType).Build()
	boxGeneric := typ.NewGeneric("Box", []*typ.TypeParam{elemType}, boxType)
	containerManifest.DefineType("Box", boxGeneric)

	// Export with function that returns generic
	containerManifest.SetExport(typ.NewRecord().
		Field("wrap", typ.Func().Param("v", typ.Any).Returns(typ.Any).Build()).
		Build())

	consumerSource := `
		local container = require("container")
		local b = container.wrap(42)
	`

	result := testutil.Check(consumerSource, testutil.WithStdlib(), testutil.WithManifest("container", containerManifest))
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("expected no errors with generic manifest")
	}
}

// TestE2E_KeysCollectorCrossModule tests that KeyOf constraint propagates via manifest
// and enables non-optional indexing in the consumer module.
func TestE2E_KeysCollectorCrossModule(t *testing.T) {
	// Create util manifest with sorted_keys function and KeyOf in summary
	utilManifest := io.NewManifest("util")

	// Function signature
	sortedKeysFn := typ.Func().
		Param("t", typ.Any).
		Returns(typ.NewArray(typ.Any)).
		Build()
	utilManifest.SetExport(typ.NewRecord().Field("sorted_keys", sortedKeysFn).Build())

	// Add summary with KeyOf constraint: returns keys of param 0
	summary := io.NewSummary([]typ.Type{typ.Any}, []typ.Type{typ.NewArray(typ.Any)})
	keyOf := constraint.KeyOf{
		Table: constraint.ParamPath(0),
		Key:   constraint.RetPath(0),
	}
	summary.Ensures = constraint.FromConstraints(keyOf)
	utilManifest.DefineSummary("sorted_keys", summary)

	// Verify KeyOf is in the enriched export
	enriched := utilManifest.EnrichedExport()
	if enriched == nil {
		t.Fatal("expected enriched export")
	}
	rec, ok := enriched.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", enriched)
	}
	fnField := rec.GetField("sorted_keys")
	if fnField == nil {
		t.Fatal("expected sorted_keys field")
	}
	fn, ok := fnField.Type.(*typ.Function)
	if !ok {
		t.Fatalf("expected Function, got %T", fnField)
	}
	if fn.Refinement == nil {
		t.Fatal("expected refinement on sorted_keys")
	}
	eff, ok := fn.Refinement.(*constraint.FunctionRefinement)
	if !ok {
		t.Fatalf("expected FunctionRefinement, got %T", fn.Refinement)
	}
	if paramIdx := eff.KeysCollectorParamIndex(); paramIdx != 0 {
		t.Errorf("KeysCollectorParamIndex: got %d, want 0", paramIdx)
	}

	// Encode and decode manifest to verify serialization works
	data, err := utilManifest.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := io.DecodeManifest(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// Verify KeyOf survives round-trip
	decodedSummary, found := decoded.LookupSummary("sorted_keys")
	if !found {
		t.Fatal("decoded manifest missing sorted_keys summary")
	}
	if !decodedSummary.Ensures.HasConstraints() {
		t.Fatal("decoded summary should have Ensures constraints")
	}

	// Verify the decoded effect
	decodedEnriched := decoded.EnrichedExport()
	decodedRec, _ := decodedEnriched.(*typ.Record)
	decodedFn, _ := decodedRec.GetField("sorted_keys").Type.(*typ.Function)
	if decodedFn.Refinement == nil {
		t.Fatal("decoded function missing refinement")
	}
	decodedEff, _ := decodedFn.Refinement.(*constraint.FunctionRefinement)
	if paramIdx := decodedEff.KeysCollectorParamIndex(); paramIdx != 0 {
		t.Errorf("decoded KeysCollectorParamIndex: got %d, want 0", paramIdx)
	}
}

// TestE2E_KeysCollectorAutoExport tests that keys collector pattern is automatically
// detected in Lua source and exported to manifest.
func TestE2E_KeysCollectorAutoExport(t *testing.T) {
	source := `
		local M = {}

		function M.sorted_keys(tbl)
			local keys = {}
			for k, _ in pairs(tbl) do
				table.insert(keys, k)
			end
			return keys
		end

		return M
	`

	result := testutil.CheckAndExport(source, "util", testutil.WithStdlib())
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Fatal("expected no errors")
	}

	summary, found := result.Manifest.LookupSummary("sorted_keys")
	if !found {
		t.Fatal("expected sorted_keys summary in manifest")
	}
	if !summary.Ensures.HasConstraints() {
		t.Fatal("expected Ensures constraints on sorted_keys summary")
	}

	constraints := summary.Ensures.AllConstraints()
	var foundKeyOf bool
	for _, c := range constraints {
		if keyOf, ok := c.(constraint.KeyOf); ok {
			foundKeyOf = true
			if !keyOf.Table.IsPlaceholder() || keyOf.Table.PlaceholderIndex() != 0 {
				t.Errorf("KeyOf.Table: expected $0, got %s", keyOf.Table.String())
			}
			if !constraint.IsReturnPath(keyOf.Key) {
				t.Errorf("KeyOf.Key: expected ret[0], got %s", keyOf.Key.String())
			}
		}
	}
	if !foundKeyOf {
		t.Error("expected KeyOf constraint in sorted_keys summary")
	}

	enriched := result.Manifest.EnrichedExport()
	rec, ok := enriched.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", enriched)
	}
	fn, ok := rec.GetField("sorted_keys").Type.(*typ.Function)
	if !ok {
		t.Fatal("expected Function type")
	}
	if fn.Refinement == nil {
		t.Fatal("expected refinement on sorted_keys")
	}
	eff, ok := fn.Refinement.(*constraint.FunctionRefinement)
	if !ok {
		t.Fatalf("expected FunctionRefinement, got %T", fn.Refinement)
	}
	if paramIdx := eff.KeysCollectorParamIndex(); paramIdx != 0 {
		t.Errorf("KeysCollectorParamIndex: got %d, want 0", paramIdx)
	}
}

func TestE2E_KeysCollectorAutoExport_MultiReturnKeySlot(t *testing.T) {
	source := `
		local M = {}

		function M.sorted_keys(tbl)
			local keys = {}
			for k in pairs(tbl) do
				table.insert(keys, k)
			end
			return nil, keys
		end

		return M
	`

	result := testutil.CheckAndExport(source, "util", testutil.WithStdlib())
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Fatal("expected no errors")
	}

	summary, found := result.Manifest.LookupSummary("sorted_keys")
	if !found {
		t.Fatal("expected sorted_keys summary in manifest")
	}
	if !summary.Ensures.HasConstraints() {
		t.Fatal("expected Ensures constraints on sorted_keys summary")
	}

	constraints := summary.Ensures.AllConstraints()
	var foundKeyOf bool
	for _, c := range constraints {
		keyOf, ok := c.(constraint.KeyOf)
		if !ok {
			continue
		}
		foundKeyOf = true
		if !keyOf.Table.IsPlaceholder() || keyOf.Table.PlaceholderIndex() != 0 {
			t.Errorf("KeyOf.Table: expected $0, got %s", keyOf.Table.String())
		}
		if !constraint.IsReturnPath(keyOf.Key) || constraint.ReturnIndexFromString(keyOf.Key.Root) != 1 {
			t.Errorf("KeyOf.Key: expected ret[1], got %s", keyOf.Key.String())
		}
	}
	if !foundKeyOf {
		t.Error("expected KeyOf constraint in sorted_keys summary")
	}

	enriched := result.Manifest.EnrichedExport()
	rec, ok := enriched.(*typ.Record)
	if !ok {
		t.Fatalf("expected Record, got %T", enriched)
	}
	fn, ok := rec.GetField("sorted_keys").Type.(*typ.Function)
	if !ok {
		t.Fatal("expected Function type")
	}
	eff, ok := fn.Refinement.(*constraint.FunctionRefinement)
	if !ok {
		t.Fatalf("expected FunctionRefinement refinement, got %T", fn.Refinement)
	}
	paramIdx, retIdx, ok := eff.KeysCollectorInfo()
	if !ok {
		t.Fatal("expected KeysCollectorInfo to be present")
	}
	if paramIdx != 0 || retIdx != 1 {
		t.Fatalf("KeysCollectorInfo = (%d, %d), want (0, 1)", paramIdx, retIdx)
	}
}
