package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// 2) Nil / Optional Narrowing

func TestNilNarrowing_NilCheckReturn(t *testing.T) {
	source := `
		function process(x: string?)
			if x == nil then
				return
			end
			local len: integer = #x
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after nil check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestNilNarrowing_NotNilCheck(t *testing.T) {
	source := `
		function process(x: string?)
			if x ~= nil then
				local len: integer = #x
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after ~= nil check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestNilNarrowing_AssertNotNil(t *testing.T) {
	source := `
		function process(x: string?)
			assert(x ~= nil)
			local len: integer = #x
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after assert, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestNilNarrowing_CorrelatedMultiReturn(t *testing.T) {
	source := `
		function get_value(): (string?, string?)
			return "value", nil
		end

		function process()
			local result, err = get_value()
			if err then
				return
			end
			if result then
				local len: integer = #result
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with correlated multi-return, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestNilNarrowing_TruthyCheck(t *testing.T) {
	source := `
		function process(x: string?)
			if x then
				local len: integer = #x
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after truthy check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestNilNarrowing_NotXThenReturn tests that `if not x then return end` narrows x to non-nil for method calls.
func TestNilNarrowing_NotXThenReturn(t *testing.T) {
	source := `
		type Obj = {method: (self: Obj) -> string}

		function f(x: Obj?)
			if not x then return end
			local s: string = x:method()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after 'if not x then return end' guard, got: %v",
			testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestNilNarrowing_XEqualsNilThenReturnMethod tests that `if x == nil then return end` narrows x for method calls.
func TestNilNarrowing_XEqualsNilThenReturnMethod(t *testing.T) {
	source := `
		type Obj = {method: (self: Obj) -> string}

		function f(x: Obj?)
			if x == nil then return end
			local s: string = x:method()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after 'if x == nil then return end' guard, got: %v",
			testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestNilNarrowing_AssertNotNilMethod tests that assert-style guards narrow for method calls.
func TestNilNarrowing_AssertNotNilMethod(t *testing.T) {
	source := `
		type Obj = {method: (self: Obj) -> string}

		function f(x: Obj?)
			assert(x ~= nil)
			local s: string = x:method()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after 'assert(x ~= nil)' guard, got: %v",
			testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestNilNarrowing_MustFailWithoutGuard verifies that calling method on optional without guard fails.
func TestNilNarrowing_MustFailWithoutGuard(t *testing.T) {
	source := `
		type Obj = {method: (self: Obj) -> string}

		function f(x: Obj?)
			local s: string = x:method()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Errorf("expected error when calling method on optional without guard")
	}
}
