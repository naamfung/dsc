package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestCallAritySoundness_UnknownParamIsRequired(t *testing.T) {
	code := `
		local function f(x: unknown): number
			return 1
		end
		local y = f()
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected error for missing required unknown parameter")
	}
}

func TestCallAritySoundness_RequiredAfterOptionalStillRequired(t *testing.T) {
	code := `
		local function f(a: number?, b: number): number
			return b
		end
		local y = f(1)
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected error when omitting required param after optional")
	}
}

func TestCallAritySoundness_ColonMethodStillWorks(t *testing.T) {
	code := `
		local T = {}
		function T:foo(x: number): number
			return x
		end
		local n: number = T:foo(1)
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCallAritySoundness_DotCallOnColonMethodFails(t *testing.T) {
	code := `
		local T = { value = 5 }
		function T:foo(x: number): number
			return self.value + x
		end
		local n: number = T.foo(1)
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected error for dot call on colon-defined method")
	}
}

func TestCallAritySoundness_ColonCallOnDotFunctionFails(t *testing.T) {
	code := `
		local T = {}
		function T.foo(x: number): number
			return x + 1
		end
		local n: number = T:foo(1)
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected error for colon call on dot-defined function without self")
	}
}

func TestCallAritySoundness_ColonCallOnFieldAssignedFunctionFails(t *testing.T) {
	code := `
		local T = {}
		T.foo = function(x: number): number
			return x + 1
		end
		local n: number = T:foo(1)
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected error for colon call on field-assigned function without self")
	}
}
