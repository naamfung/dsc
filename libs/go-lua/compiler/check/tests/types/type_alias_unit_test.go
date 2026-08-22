package types

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

// TestTypeAliasStructuralEquivalence tests that type aliases are structurally
// equivalent to their underlying types.
func TestTypeAliasStructuralEquivalence(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "string_alias_accepts_string_literal",
			code: `
				type Name = string
				local x: Name = "world"
			`,
			wantError: false,
		},
		{
			name: "function_alias_accepts_function_literal",
			code: `
				type Handler = fun(s: string): string
				local f: Handler = function(s: string): string return s end
			`,
			wantError: false,
		},
		{
			name: "string_alias_rejects_number",
			code: `
				type Name = string
				local x: Name = 42
			`,
			wantError: true,
		},
		{
			name: "function_alias_rejects_wrong_signature",
			code: `
				type Handler = fun(s: string): string
				local f: Handler = function(n: number): number return n end
			`,
			wantError: true,
		},
		{
			name: "nested_alias_chain",
			code: `
				type A = string
				type B = A
				type C = B
				local x: C = "hello"
			`,
			wantError: false,
		},
		{
			name: "record_alias_accepts_table",
			code: `
				type Point = {x: number, y: number}
				local p: Point = {x = 1, y = 2}
			`,
			wantError: false,
		},
		{
			name: "generic_function_alias_callable",
			code: `
				type Mapper<T, U> = fun(x: T): U
				local function apply<T, U>(f: Mapper<T, U>, v: T): U
					return f(v)
				end
				local result = apply(function(s: string): number return #s end, "hello")
				local n: number = result
			`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code)
			if result.HasError() != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// TestTypeAliasNestedFunctionScoping tests that chunk-level type aliases
// are visible inside nested functions.
func TestTypeAliasNestedFunctionScoping(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "alias_used_in_nested_function_call",
			code: `
type Name = string

local function greet(name: Name): string
    return "Hello, " .. name
end

local function main(): boolean
    local msg: string = greet("world")
    return true
end
`,
			wantError: false,
		},
		{
			name: "alias_used_in_nested_function_param_return",
			code: `
type Name = string

local function main(name: Name): Name
    return name
end

local function outer(): boolean
    local s: string = main("ok")
    return true
end
`,
			wantError: false,
		},
		{
			name: "alias_at_chunk_level_control",
			code: `
type Name = string

local function greet(name: Name): string
    return "Hello, " .. name
end

local msg = greet("world")
`,
			wantError: false,
		},
		{
			name: "multiple_aliases_in_nested_function",
			code: `
type Name = string
type Age = number

local function describe(name: Name, age: Age): string
    return name
end

local function main(): boolean
    local s: string = describe("Alice", 30)
    return true
end
`,
			wantError: false,
		},
		{
			name: "record_alias_in_nested_function",
			code: `
type Point = {x: number, y: number}

local function origin(): Point
    return {x = 0, y = 0}
end

local function main(): boolean
    local p: Point = origin()
    return p.x == 0
end
`,
			wantError: false,
		},
		{
			name: "function_alias_in_nested_function",
			code: `
type Handler = fun(s: string): string

local function apply(h: Handler, s: string): string
    return h(s)
end

local function main(): boolean
    local result: string = apply(function(s: string): string return s end, "test")
    return true
end
`,
			wantError: false,
		},
		{
			name: "alias_in_deeply_nested_function",
			code: `
type ID = string

local function outer(): boolean
    local function middle(): boolean
        local function inner(id: ID): string
            return id
        end
        return inner("abc") == "abc"
    end
    return middle()
end
`,
			wantError: false,
		},
		{
			name: "multiple_functions_share_alias",
			code: `
type Count = number

local function add(a: Count, b: Count): Count
    return a + b
end

local function mul(a: Count, b: Count): Count
    return a * b
end

local function compute(): boolean
    local x: Count = add(1, 2)
    local y: Count = mul(x, 3)
    return y > 0
end
`,
			wantError: false,
		},
		{
			name: "alias_used_in_conditional_inside_nested",
			code: `
type Flag = boolean

local function check(f: Flag): string
    if f then
        return "yes"
    else
        return "no"
    end
end

local function main(): boolean
    local result: string = check(true)
    return true
end
`,
			wantError: false,
		},
		{
			name: "alias_used_in_loop_inside_nested",
			code: `
type Item = string

local function process(items: {Item}): number
    local count = 0
    for i, item in ipairs(items) do
        count = count + 1
    end
    return count
end

local function main(): boolean
    local n: number = process({"a", "b"})
    return n > 0
end
`,
			wantError: false,
		},
		{
			name: "alias_defined_between_functions",
			code: `
local function first(): string
    return "first"
end

type Middle = string

local function second(m: Middle): string
    return m
end

local function third(): boolean
    local s: string = second("test")
    return true
end
`,
			wantError: false,
		},
		{
			name: "union_alias_in_nested_function",
			code: `
type Result = string | nil

local function maybe(): Result
    return nil
end

local function main(): boolean
    local r: Result = maybe()
    return r == nil
end
`,
			wantError: false,
		},
		{
			name: "optional_alias_in_nested_function",
			code: `
type OptName = string?

local function greet(name: OptName): string
    if name then
        return name
    end
    return "anonymous"
end

local function main(): boolean
    local s: string = greet(nil)
    return true
end
`,
			wantError: false,
		},
		{
			name: "generic_alias_in_nested_function",
			code: `
type Wrapper<T> = {value: T}

local function wrap(s: string): Wrapper<string>
    return {value = s}
end

local function main(): boolean
    local w: Wrapper<string> = wrap("hello")
    local s: string = w.value
    return true
end
`,
			wantError: false,
		},
		{
			name: "nested_record_alias",
			code: `
type Inner = {x: number}
type Outer = {inner: Inner}

local function make(): Outer
    return {inner = {x = 1}}
end

local function main(): boolean
    local o: Outer = make()
    local n: number = o.inner.x
    return true
end
`,
			wantError: false,
		},
		{
			name: "alias_in_method_like_function",
			code: `
type State = {count: number}

local function increment(self: State): State
    return {count = self.count + 1}
end

local function main(): boolean
    local s: State = {count = 0}
    local s2: State = increment(s)
    return s2.count > 0
end
`,
			wantError: false,
		},
		{
			name: "alias_in_callback_inside_nested",
			code: `
type Callback = fun(x: number): number

local function apply(cb: Callback, x: number): number
    return cb(x)
end

local function main(): boolean
    local result: number = apply(function(x: number): number return x * 2 end, 5)
    return result == 10
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib())
			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: %s", d.Position.Line, d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// TestTypeAliasCycle tests recursive type aliases where a type references itself.
// This exposes limitations in cycle detection within the type resolver.
func TestTypeAliasCycle(t *testing.T) {
	source := `
type Node = { next: Node? }
local n: Node = { next = nil }
`

	result := testutil.Check(source, testutil.WithStdlib())

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("Error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - recursive type alias should be accepted, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
