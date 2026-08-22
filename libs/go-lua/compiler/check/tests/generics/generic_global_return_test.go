package generics

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestGenericGlobal_ReturnsInstantiatedRecord(t *testing.T) {
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
		local v: string = c.value
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestGenericGlobal_FieldAccessOnInstantiatedReturn(t *testing.T) {
	source := `
		type Pair<T> = {
			first: T,
			second: T
		}

		function make_pair<T>(a: T, b: T): Pair<T>
			return { first = a, second = b }
		end

		local p = make_pair("hello", "world")
		local s: string = p.first
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestGenericLocal_ReturnsInstantiatedRecord(t *testing.T) {
	source := `
		type Container<T> = {
			value: T,
			get: fun(self: self): T
		}

		local function make_container<T>(v: T): Container<T>
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
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestGenericGlobal_SimpleTypeParamReturn(t *testing.T) {
	source := `
		function identity<T>(x: T): T
			return x
		end
		local s: string = identity("hello")
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestGlobal_NonGenericReturnsRecord(t *testing.T) {
	source := `
		type Box = { value: string }
		function make_box(v: string): Box
			return { value = v }
		end
		local b = make_box("hello")
		local s: string = b.value
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestGenericGlobal_InferredReturnType(t *testing.T) {
	source := `
		function wrap<T>(v: T)
			return { value = v }
		end
		local w = wrap("hello")
		local s: string = w.value
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
