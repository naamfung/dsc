package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// 7) Callback Contextual Typing

func TestCallbackContext_ParameterTypeInferred(t *testing.T) {
	source := `
		function map<T, U>(arr: T[], fn: fun(x: T): U): U[]
			local result: U[] = {}
			for _, v in ipairs(arr) do
				table.insert(result, fn(v))
			end
			return result
		end

		local nums = {1, 2, 3}
		local strs = map(nums, function(x) return tostring(x) end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for callback param inference, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCallbackContext_ReturnTypeInferred(t *testing.T) {
	source := `
		function filter<T>(arr: T[], pred: fun(x: T): boolean): T[]
			local result: T[] = {}
			for _, v in ipairs(arr) do
				if pred(v) then
					table.insert(result, v)
				end
			end
			return result
		end

		local nums = {1, 2, 3, 4, 5}
		local evens = filter(nums, function(x) return x % 2 == 0 end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for callback return inference, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCallbackContext_ExplicitTypes(t *testing.T) {
	source := `
		function apply<T>(x: T, fn: fun(v: T): T): T
			return fn(x)
		end

		local result = apply(10, function(v: number): number return v * 2 end)
		local n: number = result
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for explicit callback types, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCallbackContext_MultipleParams(t *testing.T) {
	source := `
		function reduce<T, U>(arr: T[], init: U, fn: fun(acc: U, x: T): U): U
			local result = init
			for _, v in ipairs(arr) do
				result = fn(result, v)
			end
			return result
		end

		local nums = {1, 2, 3}
		local sum = reduce(nums, 0, function(acc, x) return acc + x end)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for multi-param callback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCallbackContext_NamedCallbackParameterTypeInferred(t *testing.T) {
	source := `
		function with_number(fn: fun(x: number): nil)
			fn(1)
		end

		local function use(x)
			local n: number = x + 1
		end

		with_number(use)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for named callback param inference, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCallbackContext_NamedCallbackMultipleParametersInferred(t *testing.T) {
	source := `
		function with_pair(fn: fun(a: number, b: string): nil)
			fn(1, "ok")
		end

		local function use(a, b)
			local n: number = a
			local s: string = b
		end

		with_pair(use)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for named multi-param callback inference, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCallbackContext_NamedGenericCallbackParameterTypeInferred(t *testing.T) {
	source := `
		function for_each<T>(arr: T[], fn: fun(x: T): nil)
			for _, v in ipairs(arr) do
				fn(v)
			end
		end

		local function use(x)
			local n: number = x + 1
		end

		for_each({1, 2, 3}, use)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for named generic callback inference, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCallbackContext_NamedGenericCallbackAcrossNestedScope(t *testing.T) {
	source := `
		local function wrapper<T>(cb: fun(): T): T
			return cb()
		end

		local function a()
			return wrapper(b)
		end

		local function b()
			return 1
		end

		local n: number = a()
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested named generic callback inference, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
