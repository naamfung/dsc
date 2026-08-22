package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestReturnFieldMerge_TableMethodAssignment tests that when a table literal
// has methods added via field assignment, the return type includes those methods.
func TestReturnFieldMerge_TableMethodAssignment(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "builder pattern with method added after literal",
			Code: `
				local function make_builder()
					local builder = { messages = {} }
					builder.add_developer = function(self, content: string)
						return content
					end
					return builder
				end

				local b = make_builder()
				local s: string = b:add_developer("hi")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "method with explicit return type annotation",
			Code: `
				local function make_builder()
					local builder = { messages = {} }
					builder.add_developer = function(self, content: string): string
						return content
					end
					return builder
				end

				local b = make_builder()
				local s: string = b:add_developer("hi")
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "method with self parameter - no field access",
			Code: `
				local function create_greeter()
					local obj = { greeting = "Hello" }
					obj.greet = function(self, name: string): string
						return name
					end
					return obj
				end

				local g = create_greeter()
				local s: string = g:greet("World")
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestReturnFieldMerge_ExactRepro is the exact repro case from the task spec.
func TestReturnFieldMerge_ExactRepro(t *testing.T) {
	code := `
		local function make_builder()
			local builder = { messages = {} }
			builder.add_developer = function(self, content: string)
				return content
			end
			return builder
		end

		local b = make_builder()
		local s: string = b:add_developer("hi")
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestReturnFieldMerge_MethodNotFound tests calling a method that should exist after merge.
func TestReturnFieldMerge_MethodNotFound(t *testing.T) {
	// This tests checking that the method exists on the return type
	code := `
		local function make_counter()
			local obj = { count = 0 }
			obj.increment = function(self)
				self.count = self.count + 1
			end
			return obj
		end

		local c = make_counter()
		c:increment()
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for method call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestReturnFieldMerge_FieldAssignedAfterLiteral tests that field assignments
// after the literal are included in the return type.
func TestReturnFieldMerge_FieldAssignedAfterLiteral(t *testing.T) {
	code := `
		local function make_obj()
			local o = {}
			o.value = 42
			return o
		end

		local x = make_obj()
		local n: number = x.value
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestReturnFieldMerge_CrossFunctionCall tests methods added to returned table are visible
// when that table is passed between functions.
func TestReturnFieldMerge_CrossFunctionCall(t *testing.T) {
	code := `
		local function create_service()
			local svc = { name = "test" }
			svc.start = function(self)
				return true
			end
			return svc
		end

		local function use_service(s)
			s:start()
		end

		local service = create_service()
		use_service(service)
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestReturnFieldMerge_NestedFunctionReturns tests nested factory function scenarios.
func TestReturnFieldMerge_NestedFunctionReturns(t *testing.T) {
	code := `
		local function outer()
			local function inner()
				local obj = { x = 1 }
				obj.get_x = function(self): number
					return self.x
				end
				return obj
			end
			return inner()
		end

		local o = outer()
		local n: number = o:get_x()
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestReturnFieldMerge_SelfFieldAccess tests that self.x returns the correct type.
func TestReturnFieldMerge_SelfFieldAccess(t *testing.T) {
	code := `
		local obj = { x = 1 }
		obj.get_x = function(self): number
			return self.x
		end
		local n: number = obj:get_x()
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestReturnFieldMerge_TableLiteralMethodSelfAccess tests self access in table literal methods.
func TestReturnFieldMerge_TableLiteralMethodSelfAccess(t *testing.T) {
	code := `
		local obj = {
			x = 1,
			get_x = function(self): number
				return self.x
			end
		}
		local n: number = obj:get_x()
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestReturnFieldMerge_ClosureAssignedMethod tests method assignment inside a nested closure.
func TestReturnFieldMerge_ClosureAssignedMethod(t *testing.T) {
	code := `
		local function make()
			local obj = { x = 1 }
			local function init()
				obj.get_x = function(self): number
					return self.x
				end
			end
			init()
			return obj
		end
		local o = make()
		local n: number = o:get_x()
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestReturnFieldMerge_ModuleImport ensures exported tables preserve method types across module boundary.
func TestReturnFieldMerge_ModuleImport(t *testing.T) {
	mod := testutil.CheckAndExport(`
		local function make_builder()
			local builder = { messages = {} }
			builder.add_developer = function(self, content: string)
				return content
			end
			return builder
		end
		return { new = make_builder }
	`, "builder", testutil.WithStdlib())
	if mod.HasError() {
		t.Fatalf("module build failed: %v", testutil.ErrorMessages(mod.Errors))
	}

	code := `
		local builder = require("builder")
		local b = builder.new()
		local s: string = b:add_developer("hi")
	`
	result := testutil.Check(code, testutil.WithStdlib(), testutil.WithModule("builder", mod))
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestParamHintsSeesEnrichedReturns verifies that param hints are computed
// using enriched return types (with field merges applied), not raw returns.
// This test fails with the timing bug (param hints see {} instead of {value: number}).
func TestParamHintsSeesEnrichedReturns(t *testing.T) {
	code := `
		local function make_obj()
			local obj = {}
			obj.value = 42
			return obj
		end

		local function use_obj(o)
			local n: number = o.value
		end

		use_obj(make_obj())
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestParamHintsSeesEnrichedReturns_Method verifies method calls work through param hints.
func TestParamHintsSeesEnrichedReturns_Method(t *testing.T) {
	code := `
		local function make_obj()
			local obj = {}
			obj.get_value = function(self): number
				return 42
			end
			return obj
		end

		local function use_obj(o)
			local n: number = o:get_value()
		end

		use_obj(make_obj())
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestReturnFieldMerge_CallbackInvokedNestedFunction verifies callback-spec calls
// (for example coroutine.spawn) propagate captured nested field assignments.
func TestReturnFieldMerge_CallbackInvokedNestedFunction(t *testing.T) {
	code := `
		local function make_obj()
			local obj = {}
			local function install()
				obj.get_value = function(self): number
					return 42
				end
			end
			coroutine.spawn(install)
			return obj
		end

		local o = make_obj()
		local n: number = o:get_value()
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestReturnFieldMerge_CallbackInvokedInlineFunction(t *testing.T) {
	code := `
		local function make_obj()
			local obj = {}
			coroutine.spawn(function()
				obj.get_value = function(self): number
					return 42
				end
			end)
			return obj
		end

		local o = make_obj()
		local n: number = o:get_value()
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
