package generics

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// 4) Generic Instantiation

func TestGenericInstantiation_IdentityFunction(t *testing.T) {
	source := `
		function identity<T>(x: T): T
			return x
		end

		local s: string = identity("hello")
		local n: number = identity(42)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for identity function, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestGenericInstantiation_GenericReturnInRecord(t *testing.T) {
	source := `
		type Container<T> = {
			value: T,
			get: fun(self: self): T
		}

		function make_container<T>(v: T): Container<T>
			return {
				value = v,
				get = function(self): T return self.value end
			}
		end

		local c = make_container("hello")
		local s: string = c:get()
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for generic container, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestGenericInstantiation_MethodOnInstantiated(t *testing.T) {
	source := `
		type Box<T> = interface {
			unwrap: fun(self: self): T
		}

		function process<T>(box: Box<T>): T
			return box:unwrap()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for method on instantiated, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestGenericInstantiation_InterfaceWithSelf(t *testing.T) {
	source := `
		type Cloneable = interface {
			clone: fun(self: self): self
		}

		function duplicate(c: Cloneable): Cloneable
			return c:clone()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for interface with self, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestGenericInstantiation_WrongType(t *testing.T) {
	source := `
		function identity<T>(x: T): T
			return x
		end

		local s: string = identity(42)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Errorf("expected error for wrong type assignment")
	}
}
