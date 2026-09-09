package tables

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// 5) Dynamic Table Indexing

func TestDynamicIndex_StringKeyAssignment(t *testing.T) {
	source := `
		local t: {[string]: number} = {}
		t["a"] = 1
		local v = t["a"]
		if v ~= nil then
			local n: number = v
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for string key assignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_DotAccessAfterBracket(t *testing.T) {
	source := `
		local t: {a: number} = {a = 1}
		local n1: number = t["a"]
		local n2: number = t.a
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for dot access after bracket, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_NonConstKey(t *testing.T) {
	source := `
		local t: {[string]: number} = {}
		local key = "a"
		t[key] = 1
		local v = t[key]
		if v ~= nil then
			local n: number = v
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for non-const key, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_IntegerKey(t *testing.T) {
	source := `
		local arr: {number} = {1, 2, 3}
		local n: number = arr[1]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for integer key, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_MixedAccess(t *testing.T) {
	source := `
		local t: {name: string, value: number} = {name = "test", value = 42}
		local s: string = t.name
		local n: number = t["value"]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for mixed access, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Computed index expressions

func TestDynamicIndex_LengthOperator(t *testing.T) {
	source := `
		local arr: {number} = {1, 2, 3}
		local n: number? = arr[#arr]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for length operator index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_LengthOperatorOnArray(t *testing.T) {
	source := `
		local arr: number[] = {1, 2, 3}
		local n: number? = arr[#arr]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for length operator on array, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_ArithmeticExpression(t *testing.T) {
	source := `
		local arr: {number} = {1, 2, 3}
		local i = 1
		local n: number? = arr[i + 1]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for arithmetic expression index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_FunctionCallResult(t *testing.T) {
	source := `
		local arr: {string} = {"a", "b", "c"}
		local function getIndex(): integer
			return 2
		end
		local s: string? = arr[getIndex()]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for function call index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_MethodChainResult(t *testing.T) {
	source := `
		type Item = {value: number}
		type Container = {
			items: Item[],
			get_items: (self) -> Item[]
		}

		local function process(c: Container)
			local items = c:get_items()
			local last: Item? = items[#items]
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for method chain result indexing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_NestedLengthOperator(t *testing.T) {
	source := `
		local matrix: {{number}} = {{1, 2}, {3, 4, 5}}
		local lastRow: {number}? = matrix[#matrix]
		if lastRow then
			local lastElem: number? = lastRow[#lastRow]
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested length operator, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_UnaryMinusExpression(t *testing.T) {
	source := `
		local arr: {number} = {1, 2, 3}
		local idx = 2
		-- negative index doesn't work in Lua but type should still resolve
		local n: number? = arr[-idx]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for unary minus index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_ParenthesizedExpression(t *testing.T) {
	source := `
		local arr: {number} = {1, 2, 3}
		local i = 1
		local j = 1
		local n: number? = arr[(i + j)]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for parenthesized expression index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicIndex_KeyTypeNarrowedInTruthyGuard(t *testing.T) {
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
		local flag: boolean = true
		local suite: string | false = (flag and "core") or false
		if suite then
			suites[suite] = suites[suite] or {}
			table.insert(suites[suite], 1)
		end

		local suite_names = sorted_keys(suites)
		for _, name in ipairs(suite_names) do
			local tests = suites[name]
			local count: number = #tests
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for truthy-guarded key use, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
