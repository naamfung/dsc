package inference

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestTableInference_NestedRecords tests inferring nested record types.
func TestTableInference_NestedRecords(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "nested table literal",
			Code: `
				local config = {
					server = {
						host = "localhost",
						port = 8080
					}
				}
				local port: number = config.server.port
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "deeply nested table",
			Code: `
				local data = {
					level1 = {
						level2 = {
							level3 = {
								value = "deep"
							}
						}
					}
				}
				local v: string = data.level1.level2.level3.value
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "mixed nested types",
			Code: `
				local obj = {
					name = "test",
					count = 42,
					nested = {
						flag = true
					}
				}
				local n: string = obj.name
				local c: number = obj.count
				local f: boolean = obj.nested.flag
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTableInference_ArrayLiterals tests inferring array types from literals.
func TestTableInference_ArrayLiterals(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "homogeneous array",
			Code: `
				local arr = {1, 2, 3}
				local n: number = arr[1]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "string array",
			Code: `
				local arr = {"a", "b", "c"}
				local s: string = arr[1]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "typed array literal",
			Code: `
				local arr: {number} = {1, 2, 3}
				local n: number = arr[1]
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTableInference_TableMethods tests inference with table methods.
func TestTableInference_TableMethods(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "table.insert preserves type",
			Code: `
				local arr: {string} = {}
				table.insert(arr, "hello")
				local s: string = arr[1]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "table.remove returns element type",
			Code: `
				local arr: {string} = {"a", "b"}
				local s: string? = table.remove(arr)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "table.concat on string array",
			Code: `
				local arr: {string} = {"a", "b", "c"}
				local s: string = table.concat(arr, ", ")
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTableInference_EmptyTable tests handling of empty table literals.
func TestTableInference_EmptyTable(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "empty table with type annotation",
			Code: `
				local arr: {number} = {}
				table.insert(arr, 1)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "empty record with type annotation",
			Code: `
				type Config = {name: string, value: number}
				local cfg: Config = {name = "test", value = 42}
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTableInference_EmptyTableFields tests that fields initialized with empty tables
// are preserved in the inferred record type.
func TestTableInference_EmptyTableFields(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "record with empty array field should preserve field",
			Code: `
				local results = {
					passed = 0,
					errors = {},
				}
				table.insert(results.errors, "test error")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "record with multiple empty table fields",
			Code: `
				local results = {
					passed = 0,
					failed = 0,
					errors = {},
					suite_times = {},
				}
				table.insert(results.errors, "error1")
				results.suite_times["test1"] = 1.5
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "empty table field access should not error",
			Code: `
				local obj = {
					items = {},
					count = 0,
				}
				local first = obj.items[1]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "table.insert on empty table field in record",
			Code: `
				local results = {
					passed = 0,
					failed = 0,
					errors = {},
				}
				table.insert(results.errors, {id = "test", error = "msg"})
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "empty table fields in function scope",
			Code: `
				local function run_tests()
					local results = {
						passed = 0,
						failed = 0,
						errors = {},
						suite_times = {},
					}

					results.passed = results.passed + 1
					table.insert(results.errors, {id = "test1", error = "failed"})
					results.suite_times["suite1"] = 1.5

					return results
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "empty table fields with conditional mutation",
			Code: `
				local function process()
					local results = {
						passed = 0,
						failed = 0,
						errors = {},
					}

					local success = true
					if success then
						results.passed = results.passed + 1
					else
						results.failed = results.failed + 1
						table.insert(results.errors, {id = "x", error = "y"})
					end

					return results
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTableInference_RecordField tests accessing record fields.
func TestTableInference_RecordField(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "direct field access",
			Code: `
				local obj = {name = "test"}
				local n: string = obj.name
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "bracket field access",
			Code: `
				local obj = {name = "test"}
				local n: string = obj["name"]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "wrong field type fails",
			Code: `
				local obj = {name = "test"}
				local n: number = obj.name
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "missing field fails",
			Code: `
				local obj = {name = "test"}
				local v = obj.missing
			`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTableInference_TypedRecords tests records with type definitions.
func TestTableInference_TypedRecords(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "typed record assignment",
			Code: `
				type Person = {name: string, age: number}
				local p: Person = {name = "Alice", age = 30}
				local n: string = p.name
				local a: number = p.age
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "typed record missing field",
			Code: `
				type Person = {name: string, age: number}
				local p: Person = {name = "Alice"}
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "typed record wrong field type",
			Code: `
				type Person = {name: string, age: number}
				local p: Person = {name = "Alice", age = "thirty"}
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "typed record with optional field",
			Code: `
				type Config = {name: string, debug: boolean?}
				local c1: Config = {name = "prod"}
				local c2: Config = {name = "dev", debug = true}
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "typed record optional field from nilable expression",
			Code: `
				type Config = {name: string, debug: boolean?}
				local maybe_debug: boolean? = nil
				local c: Config = {name = "prod", debug = maybe_debug}
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "typed record with computed key rejects",
			Code: `
				type Config = {name: string, debug: boolean?}
				local key: string = "debug"
				local c: Config = {name = "prod", [key] = true}
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "union of records with optional field",
			Code: `
				type A = {name: string}
				type B = {name: string, debug: boolean?}
				local c: A | B = {name = "prod"}
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestTableInference_MapTypes tests map type inference.
func TestTableInference_MapTypes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "string to number map lookup is optional",
			Code: `
				local scores: {[string]: number} = {["alice"] = 100, ["bob"] = 90}
				local s: number? = scores["alice"]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "map iteration with pairs",
			Code: `
				local scores: {[string]: number} = {["alice"] = 100}
				for k, v in pairs(scores) do
					local name: string = k
					local score: number = v
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "number to string map with numeric literal keys",
			Code: `
				local lookup: {[number]: string} = { [1] = "one", [2] = "two" }
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
