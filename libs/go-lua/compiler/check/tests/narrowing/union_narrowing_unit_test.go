package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// 1) Flow Narrowing Core

func TestUnionNarrowing_EqualityTag(t *testing.T) {
	source := `
		type A = {tag: "a", value: string}
		type B = {tag: "b", value: number}
		local r: A | B = {tag="a", value="x"}

		if r.tag == "a" then
			local s: string = r.value
		else
			local n: number = r.value
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestUnionNarrowing_ElseBranchWrongType(t *testing.T) {
	source := `
		type A = {tag: "a", value: string}
		type B = {tag: "b", value: number}
		local r: A | B = {tag="a", value="x"}

		if r.tag == "a" then
		else
			local s: string = r.value
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Errorf("expected error (assigning number to string), got none")
	}
}

func TestUnionNarrowing_BooleanDiscriminant(t *testing.T) {
	source := `
		type OK = {ok: true, value: string}
		type ERR = {ok: false, value: number}
		local r: OK | ERR = {ok=true, value="x"}

		if r.ok then
			local s: string = r.value
		else
			local n: number = r.value
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestUnionNarrowing_NestedField(t *testing.T) {
	source := `
		type A = {payload: {kind: "a", data: string}}
		type B = {payload: {kind: "b", data: number}}
		local r: A | B = {payload = {kind = "a", data = "x"}}

		if r.payload.kind == "a" then
			local s: string = r.payload.data
		else
			local n: number = r.payload.data
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestUnionNarrowing_NegatedCheck(t *testing.T) {
	source := `
		type OK = {ok: true, value: string}
		type ERR = {ok: false, error: string}
		local r: OK | ERR = {ok=true, value="x"}

		if not r.ok then
			local e: string = r.error
		else
			local v: string = r.value
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// 2) Never Type Regression Tests
// These test that variables don't incorrectly narrow to "never" in non-exhaustive branches.

// TestUnionNarrowing_NegatedTagShouldNotBeNever tests that negated equality
// does not cause the type to become "never".
// Pattern: if r.tag ~= "a" then ... r should still be callable
func TestUnionNarrowing_NegatedTagShouldNotBeNever(t *testing.T) {
	source := `
		type A = {tag: "a", f: fun(): number}
		type B = {tag: "b", f: fun(): number}

		local function get(): A | B
			return {tag = "a", f = function(): number return 1 end}
		end

		local r: A | B = get()
		if r.tag ~= "a" then
			local x: number = r.f()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("negated tag check should not cause never type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_NegatedTagCallInBranch tests that function calls work
// inside negated tag branches.
func TestUnionNarrowing_NegatedTagCallInBranch(t *testing.T) {
	source := `
		type A = {tag: "a", process: fun(): string}
		type B = {tag: "b", process: fun(): string}

		local function get(): A | B
			return {tag = "b", process = function(): string return "ok" end}
		end

		local r: A | B = get()
		if r.tag ~= "a" then
			local s: string = r:process()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("method call in negated branch should work, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_MultiBranchNonExhaustive tests that the else branch
// in a multi-way if does not become "never" when branches are non-exhaustive.
func TestUnionNarrowing_MultiBranchNonExhaustive(t *testing.T) {
	source := `
		type A = {tag: "a", f: fun(): number}
		type B = {tag: "b", f: fun(): number}
		type C = {tag: "c", f: fun(): number}

		local function get(): A | B | C
			return {tag = "a", f = function(): number return 1 end}
		end

		local r: A | B | C = get()
		if r.tag == "a" then
			local x: number = r.f()
		elseif r.tag == "b" then
			local x: number = r.f()
		else
			local x: number = r.f()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("else branch should not become never, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_ElseBranchWithTwoVariants tests that else branch
// retains correct type when only one of multiple variants is eliminated.
func TestUnionNarrowing_ElseBranchWithTwoVariants(t *testing.T) {
	source := `
		type A = {tag: "a", value: string}
		type B = {tag: "b", value: number}
		type C = {tag: "c", value: boolean}

		local function get(): A | B | C
			return {tag = "b", value = 42}
		end

		local r: A | B | C = get()
		if r.tag == "a" then
			local s: string = r.value
		else
			-- r should be B | C here, both have .value field
			local v = r.value
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("else branch should retain B|C, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_EarlyReturnNarrows tests that after an early return,
// the remaining code sees the narrowed type (not never).
func TestUnionNarrowing_EarlyReturnNarrows(t *testing.T) {
	source := `
		type A = {tag: "a", value: string}
		type B = {tag: "b", count: number}

		local function get(): A | B
			return {tag = "b", count = 42}
		end

		local function process(): number
			local r: A | B = get()
			if r.tag == "a" then
				return 0
			end
			-- r should be B here, not never
			local n: number = r.count
			return n
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("after early return, r should be B not never, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_EarlyReturnWithMethodCall tests that method calls work
// after early return narrows the type.
func TestUnionNarrowing_EarlyReturnWithMethodCall(t *testing.T) {
	source := `
		type A = {tag: "a", get: fun(): string}
		type B = {tag: "b", get: fun(): number}

		local function make(): A | B
			return {tag = "b", get = function(): number return 42 end}
		end

		local function process(): number
			local r: A | B = make()
			if r.tag == "a" then
				return 0
			end
			-- r should be B here
			local n: number = r:get()
			return n
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("method call after early return should work, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_ChainedEarlyReturns tests multiple early returns
// progressively narrowing the type.
func TestUnionNarrowing_ChainedEarlyReturns(t *testing.T) {
	source := `
		type A = {tag: "a"}
		type B = {tag: "b"}
		type C = {tag: "c", data: number}

		local function get(): A | B | C
			return {tag = "c", data = 100}
		end

		local function process(): number
			local r: A | B | C = get()
			if r.tag == "a" then return 0 end
			if r.tag == "b" then return 1 end
			-- r should be C here
			local n: number = r.data
			return n
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("chained early returns should narrow correctly, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_NilCheckDoesNotMakeNever tests that nil checks
// don't cause the non-nil branch to become never.
func TestUnionNarrowing_NilCheckDoesNotMakeNever(t *testing.T) {
	source := `
		type Result = {value: string} | nil

		local function get(): Result
			return {value = "hello"}
		end

		local function process(): string
			local r: Result = get()
			if r == nil then
				return ""
			end
			-- r should be {value: string} here
			local s: string = r.value
			return s
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("nil check should narrow to non-nil type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_FieldAccessAfterNegatedCheck tests that field access
// works after a negated equality check.
func TestUnionNarrowing_FieldAccessAfterNegatedCheck(t *testing.T) {
	source := `
		type A = {kind: "a", data: string}
		type B = {kind: "b", data: number}

		local function get(): A | B
			return {kind = "b", data = 42}
		end

		local r: A | B = get()
		if r.kind ~= "a" then
			-- r should be B here
			local n: number = r.data
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("field access after negated check should work, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_CommonMethodOnAllVariants tests that a method present
// on all union variants remains callable after partial narrowing.
func TestUnionNarrowing_CommonMethodOnAllVariants(t *testing.T) {
	source := `
		type A = {tag: "a", call: fun(): string}
		type B = {tag: "b", call: fun(): string}
		type C = {tag: "c", call: fun(): string}

		local function get(): A | B | C
			return {tag = "a", call = function(): string return "a" end}
		end

		local r: A | B | C = get()
		if r.tag ~= "a" then
			-- r is B | C, both have call method
			local s: string = r:call()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("common method should be callable on narrowed union, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_FunctionFieldAfterNarrowing tests that function fields
// remain callable after union narrowing (guards against "expected function, got never").
func TestUnionNarrowing_FunctionFieldAfterNarrowing(t *testing.T) {
	source := `
		type Handler = {
			kind: "handler",
			process: fun(x: number): number
		}
		type Fallback = {
			kind: "fallback",
			process: fun(x: number): number
		}

		local function get(): Handler | Fallback
			return {kind = "handler", process = function(x: number): number return x end}
		end

		local h: Handler | Fallback = get()
		if h.kind == "handler" then
			local result: number = h.process(42)
		else
			local result: number = h.process(0)
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("function field should be callable after narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_SequentialNarrowingDoesNotMakeNever tests that
// sequential narrowing checks don't incorrectly produce "never".
func TestUnionNarrowing_SequentialNarrowingDoesNotMakeNever(t *testing.T) {
	source := `
		type A = {kind: "a"}
		type B = {kind: "b"}
		type C = {kind: "c"}

		local function get(): A | B | C
			return {kind = "b"}
		end

		local function test()
			local r: A | B | C = get()

			if r.kind == "a" then
				return "is a"
			end

			-- r should be B | C, not never
			if r.kind == "b" then
				return "is b"
			end

			-- r should be C, not never
			return "is c"
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("sequential narrowing should not produce never, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_LoopWithNarrowingDoesNotMakeNever tests that
// narrowing inside loops doesn't incorrectly produce "never".
func TestUnionNarrowing_LoopWithNarrowingDoesNotMakeNever(t *testing.T) {
	source := `
		type Event = {kind: "event", data: string}
		type Timeout = {kind: "timeout"}

		local function get(): Event | Timeout
			return {kind = "event", data = "test"}
		end

		local function process()
			while true do
				local r: Event | Timeout = get()
				if r.kind == "timeout" then
					break
				end
				-- r should be Event here
				local s: string = r.data
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("narrowing inside loop should work, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_ReassignmentAfterNarrowing tests that reassigning
// a variable after narrowing doesn't cause "never" type issues.
func TestUnionNarrowing_ReassignmentAfterNarrowing(t *testing.T) {
	source := `
		type A = {tag: "a", x: number}
		type B = {tag: "b", y: string}

		local function getA(): A
			return {tag = "a", x = 1}
		end

		local function getAB(): A | B
			return {tag = "b", y = "test"}
		end

		local function test()
			local r: A | B = getAB()
			if r.tag == "a" then
				local n: number = r.x
			end
			-- After the if, r is still A | B (not narrowed)
			r = getA()
			-- Now r is A
			local n: number = r.x
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("reassignment after narrowing should work, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_MethodCallOnNarrowedReceiver tests that method calls
// work correctly on narrowed union receivers.
func TestUnionNarrowing_MethodCallOnNarrowedReceiver(t *testing.T) {
	source := `
		type Success = {
			ok: true,
			getValue: fun(): string
		}
		type Failure = {
			ok: false,
			getError: fun(): string
		}

		local function get(): Success | Failure
			return {ok = true, getValue = function(): string return "value" end}
		end

		local function test(): string
			local r: Success | Failure = get()
			if r.ok then
				return r:getValue()
			else
				return r:getError()
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("method call on narrowed receiver should work, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestUnionNarrowing_NestedFunctionWithNarrowing tests that narrowing
// works correctly inside nested functions.
func TestUnionNarrowing_NestedFunctionWithNarrowing(t *testing.T) {
	source := `
		type A = {tag: "a", data: string}
		type B = {tag: "b", data: number}

		local function get(): A | B
			return {tag = "a", data = "hello"}
		end

		local function outer()
			local r: A | B = get()

			local function inner(): string
				if r.tag == "a" then
					return r.data
				end
				return ""
			end

			return inner()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("narrowing in nested function should work, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
