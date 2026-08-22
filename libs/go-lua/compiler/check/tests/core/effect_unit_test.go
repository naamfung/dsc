package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// 8) Effect / Predicate Link Propagation

func TestEffect_LocalAssertNotNil(t *testing.T) {
	source := `
		function check_not_nil(x: any): boolean
			return x ~= nil
		end

		function process(x: string?)
			if check_not_nil(x) then
				-- Cannot narrow without effect annotation
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	// Should not error - just checking that it compiles
	if result.HasError() {
		t.Errorf("expected no errors for local assert, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEffect_TerminationAssertFalse(t *testing.T) {
	source := `
		function fail_fast()
			assert(false)
			local unreachable = "this should not matter"
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	// Should not error - code after assert(false) is unreachable
	if result.HasError() {
		t.Errorf("expected no errors after assert(false), got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEffect_ErrorTerminates(t *testing.T) {
	source := `
		function validate(x: string?)
			if x == nil then
				error("x is required")
			end
			local len: integer = #x
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after error() terminates, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestEffect_TypePredicateIs(t *testing.T) {
	source := `
		type Dog = {kind: "dog", bark: fun(self: self)}
		type Cat = {kind: "cat", meow: fun(self: self)}

		function is_dog(x: Dog | Cat): boolean
			return x.kind == "dog"
		end

		function process(pet: Dog | Cat)
			if pet.kind == "dog" then
				pet:bark()
			else
				pet:meow()
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for type predicate, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
