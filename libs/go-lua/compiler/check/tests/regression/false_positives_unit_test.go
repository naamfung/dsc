package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// False Positive Reproductions from wippy lint
// These tests document bugs that produce false positive errors

// 1. Bracket notation on maps remains soundly optional until presence is proven.
func TestBracketNotationOnMap_GuardedAccess(t *testing.T) {
	source := `
		local method_names: {[string]: string} = {
			greet = "hello",
			farewell = "goodbye"
		}
		local maybe_name: string? = method_names["greet"]
		assert(method_names["greet"])
		local name: string = method_names["greet"]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for guarded bracket notation on map, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_BracketNotationOnRecord(t *testing.T) {
	source := `
		local config: {host: string, port: integer} = {
			host = "localhost",
			port = 8080
		}
		local h: string = config["host"]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for bracket notation on record, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// 2. E0019: Arithmetic on union with nil
// Pattern: revenues[i] - expenses[i] where values are number?
// Note: Map indexing soundly returns T? since key may not exist

func TestFalsePositive_ArithmeticOnOptionalMapElements_SoundBehavior(t *testing.T) {
	// This is expected to fail - map indexing returns T? soundly
	source := `
		local revenues: {[integer]: number} = {10000, 9500, 12000}
		local expenses: {[integer]: number} = {8000, 8500, 9000}

		for i = 1, 3 do
			local profit: number = revenues[i] - expenses[i]
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Errorf("expected error for arithmetic on optional map elements (sound behavior)")
	}
}

func TestFalsePositive_ArithmeticOnTupleElements_BoundedLoop(t *testing.T) {
	// Bounded for-loop with matching tuple length should exclude nil from index result
	source := `
		local revenues = {10000, 9500, 12000}
		local expenses = {8000, 8500, 9000}

		for i = 1, 3 do
			local profit: number = revenues[i] - expenses[i]
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for bounded tuple indexing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_TupleLiteralIndexing(t *testing.T) {
	// Literal index on tuple should return exact element type without nil
	source := `
		local data = {10, 20, 30}
		local v: number = data[1]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for literal tuple indexing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_TupleDynamicIndexing(t *testing.T) {
	// Dynamic index on tuple returns union with nil (sound but verbose)
	source := `
		local data = {10, 20, 30}

		function get(i: integer): number?
			return data[i]
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for dynamic tuple indexing returning optional, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_ArithmeticOnOptionalAfterGuard(t *testing.T) {
	source := `
		local values: {[integer]: number} = {10, 20, 30}

		function compute(i: integer): number
			local v = values[i]
			if v == nil then
				return 0
			end
			return v * 2
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after nil guard on map value, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// 3. E0002: Expected function, got never
// Pattern: After reassignment where variable is narrowed incorrectly

func TestFalsePositive_NeverAfterReassignment(t *testing.T) {
	source := `
		type Result = {ok: boolean, value: string?}

		function process(): Result
			local result: Result = {ok = true, value = nil}
			local err: string? = nil

			result, err = {ok = true, value = "first"}, nil
			if err then
				return {ok = false, value = nil}
			end

			result, err = {ok = true, value = "second"}, nil
			if err then
				return {ok = false, value = nil}
			end

			return result
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after multi-assignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_NeverAfterMultiReturnReassignment(t *testing.T) {
	source := `
		type Obj = {
			stat: (self: Obj, path: string) -> (boolean, string?),
			write: (self: Obj, data: string) -> (boolean, string?)
		}

		function process(vol: Obj)
			local ok: boolean
			local err: string?

			ok, err = vol:stat("file")
			if err then
				return
			end

			ok, err = vol:write("data")
			if err then
				return
			end

			if ok then
				print("success")
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after multi-return reassignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// 4. Reassignment kills old constraints
// Bug: assert.is_nil(vol) on first definition shouldn't affect second definition

func TestFalsePositive_ReassignmentKillsConstraints(t *testing.T) {
	source := `
		type File = {
			open: (self: File, mode: string) -> boolean
		}

		function getFile(name: string): File?
			return nil
		end

		function test()
			local vol: File? = getFile("nonexistent")
			-- Guard pattern: if not nil, return early
			if vol ~= nil then
				return
			end
			-- After guard: vol is narrowed to nil

			-- Reassignment: vol gets new value
			vol = getFile("valid")
			-- After reassignment: vol should be File? (not nil)

			-- Second guard
			if vol == nil then
				return
			end
			-- After guard: vol should be File (not nil)

			local ok: boolean = vol:open("r")
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after reassignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_ReassignmentKillsIsNilConstraint(t *testing.T) {
	source := `
		type Obj = {
			method: (self: Obj) -> string
		}

		function getObj(name: string): Obj?
			return nil
		end

		function test()
			local obj: Obj? = getObj("first")
			if obj == nil then
				-- obj is nil here
			end

			obj = getObj("second")
			if obj ~= nil then
				local s: string = obj:method()
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after reassignment kills old constraint, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Additional pattern: method call after multiple guards
func TestFalsePositive_MethodCallAfterMultipleGuards(t *testing.T) {
	source := `
		type FileSystem = {
			exists: (self: FileSystem, path: string) -> boolean,
			read: (self: FileSystem, path: string) -> (string?, string?)
		}

		function loadConfig(fs: FileSystem, path: string): string?
			if not fs:exists(path) then
				return nil
			end

			local content, err = fs:read(path)
			if err then
				return nil
			end

			return content
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for method calls with guards, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy: error() call in else branch after truthy check
// if result.process_called then ... else error("...") end
func TestFalsePositive_ErrorInElseBranchAfterTruthyCheck(t *testing.T) {
	source := `
		type Result = { process_called: boolean?, process_to_func_id: string? }

		function test(result: Result)
			if result.process_called then
				return "ok"
			else
				error("process_called marker not inherited: got " .. tostring(result.process_called))
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for error() in else branch, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy: method call on executor returns never
// exec:call("name", args) and exec:async("name", args)
func TestFalsePositive_MethodCallReturnsNever(t *testing.T) {
	source := `
		type Executor = {
			call: (self: Executor, name: string, ...any) -> (any, string?),
			async: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function test(exec: Executor)
			local result, err = exec:call("app.test.funcs:echo", "executor call")
			if err then
				error(err)
			end
			return result
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for method call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy: multiple conditional checks then error
func TestFalsePositive_MultipleConditionsThenError(t *testing.T) {
	source := `
		type Meta = { role: string?, department: string? }

		function validate(meta: Meta)
			if meta.role ~= "admin" then
				error("actor role mismatch: expected admin, got " .. tostring(meta.role))
			end
			if meta.department ~= "engineering" then
				error("actor department mismatch: expected engineering, got " .. tostring(meta.department))
			end
			return true
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for conditional error calls, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy: funcs.call returns any, field access then conditional error
// funcs.new():call() returns (any, error?) - result is any
func TestFalsePositive_AnyFieldAccessThenConditionalError(t *testing.T) {
	source := `
		type Executor = {
			call: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function test(exec: Executor)
			local result, err = exec:call("app.test.ctx:ctx_reader", { "process_to_func_id", "process_called" })
			if err then
				error("call failed: " .. tostring(err))
			end

			-- result is any, result.process_to_func_id is any
			if result.process_to_func_id ~= "ptf-321" then
				error("process_to_func_id not inherited: got " .. tostring(result.process_to_func_id))
			end

			if result.process_called ~= true then
				error("process_called marker not inherited: got " .. tostring(result.process_called))
			end

			return true
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for any field access with conditional error, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy: type() check on any-typed value makes else branch unreachable
// if x then if type(x) == "table" then ... else tostring(x) end end
func TestFalsePositive_TypeCheckOnAnyElseBranchReachable(t *testing.T) {
	source := `
		type Event = { result: any }

		function test(event: Event)
			if event.result then
				if type(event.result) == "table" then
					return "table"
				else
					return tostring(event.result)
				end
			end
			return "nil"
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for type check else branch, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy link_explicit: variable assigned in loop, nil guard, then type check
func TestFalsePositive_LoopAssignedVarTypeCheckElseBranch(t *testing.T) {
	source := `
		type Item = { pid: integer, result: any }

		function test(items: {Item})
			local found: Item? = nil
			for _, item in ipairs(items) do
				if item.pid == 123 then
					found = item
					break
				end
			end

			if not found then
				return "not found"
			end

			-- After nil guard, found should be Item (not nil)
			if found.result then
				if type(found.result) == "table" then
					return "table"
				else
					-- This should NOT be unreachable
					return tostring(found.result)
				end
			end
			return "nil result"
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for loop-assigned var type check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy link_explicit: OPTIONAL any field, truthy check, type check makes else unreachable
func TestFalsePositive_OptionalAnyFieldTypeCheckElseBranch(t *testing.T) {
	source := `
		type Event = { kind: string, result?: any }
		type Item = { pid: integer, result?: any }

		function test(event: Event)
			local item: Item = { pid = 1, result = event.result }

			if item.result then
				if type(item.result) == "table" then
					return "table"
				else
					-- This should NOT be unreachable
					return tostring(item.result)
				end
			end
			return "nil"
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for optional any field type check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy executor: method returning Self, then assert.neq comparison
func TestFalsePositive_SelfMethodThenNeqAssertion(t *testing.T) {
	source := `
		type Executor = {
			with_options: (self: Executor, opts: any) -> Executor,
			call: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function new_executor(): Executor
			return {} :: Executor
		end

		function not_nil(v: any, msg: string)
			if v == nil then error(msg) end
		end

		function neq(a: any, b: any, msg: string)
			if a == b then error(msg) end
		end

		function test()
			local exec = new_executor()
			not_nil(exec, "exec created")

			local exec2 = exec:with_options({ timeout = 1000 })
			not_nil(exec2, "with_options returns executor")
			neq(exec, exec2, "with_options returns new executor")

			-- After neq(exec, exec2), exec should NOT be narrowed to never
			local result, err = exec:call("test", "arg")
			return result
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after neq assertion, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Simplified: just the neq pattern
func TestFalsePositive_NeqAssertionSimple(t *testing.T) {
	source := `
		type Obj = { call: (self: Obj) -> any }

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("equal") end
		end

		function test()
			local x = make()
			local y = make()
			neq(x, y)
			return x:call()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after simple neq, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// With method call between make and neq
func TestFalsePositive_NeqAfterMethodCall(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> any
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("equal") end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			return x:call()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after neq with derive, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// With not_nil assertions before neq
func TestFalsePositive_NotNilThenNeq(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> any
		}

		function make(): Obj
			return {} :: Obj
		end

		function not_nil(v: any)
			if v == nil then error("nil") end
		end

		function neq(a: any, b: any)
			if a == b then error("equal") end
		end

		function test()
			local x = make()
			not_nil(x)

			local y = x:derive()
			not_nil(y)
			neq(x, y)

			return x:call()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after not_nil + neq, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Exact wippy pattern: derive returns same type, then neq with derived, then call original
func TestFalsePositive_DeriveNeqThenCallOriginal(t *testing.T) {
	source := `
		type Executor = {
			with_options: (self: Executor, opts: any) -> Executor,
			call: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function new_executor(): Executor
			return {} :: Executor
		end

		function not_nil(v: any, msg: string)
			if v == nil then error(msg) end
		end

		function neq(a: any, b: any, msg: string)
			if a == b then error(msg) end
		end

		function main()
			local exec = new_executor()
			not_nil(exec, "executor created")

			-- derive new executor from original
			local exec2 = exec:with_options({ timeout = 1000 })
			not_nil(exec2, "with_options returns executor")

			-- assert they're different objects
			neq(exec, exec2, "with_options returns new executor")

			-- call on ORIGINAL - this is where wippy fails
			local result, err = exec:call("test:echo", "arg")
			not_nil(result, "call returns result")

			-- later call async on same original
			local future, aerr = exec:call("test:echo", "arg2")
			return result
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for derive-neq-call pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Minimal: just neq then call - without not_nil assertions
func TestFalsePositive_NeqThenCall_Minimal(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> any
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("equal") end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			return x:call()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for minimal neq-then-call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// With not_nil before neq - same types as full test
func TestFalsePositive_NotNilThenNeqExact(t *testing.T) {
	source := `
		type Executor = {
			with_options: (self: Executor, opts: any) -> Executor,
			call: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function new_executor(): Executor
			return {} :: Executor
		end

		function not_nil(v: any, msg: string)
			if v == nil then error(msg) end
		end

		function neq(a: any, b: any, msg: string)
			if a == b then error(msg) end
		end

		function main()
			local exec = new_executor()
			not_nil(exec, "a")
			local exec2 = exec:with_options({})
			not_nil(exec2, "b")
			neq(exec, exec2, "c")
			return exec:call("x", "y")
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// With multi-value assignment after call
func TestFalsePositive_NotNilNeqMultiReturn(t *testing.T) {
	source := `
		type Executor = {
			with_options: (self: Executor, opts: any) -> Executor,
			call: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function new_executor(): Executor
			return {} :: Executor
		end

		function not_nil(v: any, msg: string)
			if v == nil then error(msg) end
		end

		function neq(a: any, b: any, msg: string)
			if a == b then error(msg) end
		end

		function main()
			local exec = new_executor()
			not_nil(exec, "a")
			local exec2 = exec:with_options({})
			not_nil(exec2, "b")
			neq(exec, exec2, "c")
			local result, err = exec:call("x", "y")
			return result
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with multi-return, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Simplest case: neq then multi-return call
func TestFalsePositive_NeqThenMultiReturn(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> (any, string?)
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("eq") end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			local r, e = x:call()
			return r
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Without neq - just multi-return
func TestFalsePositive_MultiReturnNoNeq(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> (any, string?)
		}

		function make(): Obj
			return {} :: Obj
		end

		function test()
			local x = make()
			local y = x:derive()
			local r, e = x:call()
			return r
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors without neq, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// neq without error() - just returns
func TestFalsePositive_NeqNoErrorMultiReturn(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> (any, string?)
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then
				print("equal")
			end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			local r, e = x:call()
			return r
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with neq without error, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// neq with error + non-optional multi-return
func TestFalsePositive_NeqErrorNonOptionalMultiReturn(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> (any, string)
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("eq") end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			local r, e = x:call()
			return r
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with non-optional, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// neq with error + single-return assignment
func TestFalsePositive_NeqErrorSingleReturn(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> any
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("eq") end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			local r = x:call()
			return r
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with single return, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Type guard else branch should be reachable when field is any
func TestFalsePositive_TypeGuardElseBranchWithAnyField(t *testing.T) {
	source := `
		local exits = {}
		table.insert(exits, {pid = 1, result = "value"})

		local worker_exit = nil
		for _, exit in ipairs(exits) do
			worker_exit = exit
			break
		end

		if not worker_exit then
			return
		end

		if worker_exit.result then
			if type(worker_exit.result) == "table" then
				print("table")
			else
				local s = tostring(worker_exit.result)
				print(s)
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with type guard else branch, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Loop assignment with conditional field access
func TestFalsePositive_LoopAssignmentThenFieldCheck(t *testing.T) {
	source := `
		type Item = {
			pid: integer,
			result: any
		}

		function test()
			local exits: {Item} = {}
			local worker_pid: integer = 1

			local worker_exit: Item? = nil
			for _, exit in ipairs(exits) do
				if exit.pid == worker_pid then
					worker_exit = exit
					break
				end
			end

			if not worker_exit then
				return false, "not found"
			end

			local result_value = worker_exit.result
			if result_value ~= "expected" then
				local result_str = "nil"
				if worker_exit.result then
					result_str = "truthy"
				else
					result_str = tostring(worker_exit.result)
				end
				return false, result_str
			end

			return true
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with loop assignment pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_AndGuardNarrowsOptional(t *testing.T) {
	source := `
		local s: {name: string}? = nil

		function test(): string?
			if s and s.name then
				return s.name
			end
			return nil
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for and-guard on optional, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_AndGuardNarrowsOptional_Expression(t *testing.T) {
	source := `
		local s: {name: string}? = nil
		local name = s and s.name or "unknown"
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for and/or expression guard, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_AndGuardNarrowsOptional_ErrorReturn(t *testing.T) {
	readerManifest := io.NewManifest("reader")
	readerManifest.SetExport(typ.NewRecord().
		Field("script_by_id", typ.Func().
			Param("id", typ.String).
			Returns(
				typ.NewOptional(typ.NewRecord().Field("name", typ.String).Build()),
				typ.NewOptional(typ.LuaError),
			).
			Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
			Build()).
		Build())

	source := `
local reader = require("reader")
local script, _ = reader.script_by_id("id")
local script_name = script and script.name or "Unknown Script"
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("reader", readerManifest))
	if result.HasError() {
		t.Errorf("expected no errors for and/or guard on error-return value, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_OrEmptyStringStaysString(t *testing.T) {
	source := `
		local s: string? = nil
		local r: string = (s or ""):sub(1, 2)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for or-empty-string, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_AndGuardNarrowsNestedPath(t *testing.T) {
	source := `
		local rec: {foo: {bar: string}?}? = nil

		function test(): string
			if rec and rec.foo and rec.foo.bar then
				return rec.foo.bar
			end
			return "x"
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested and-guard, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_AndGuardNarrowsNestedPath_Expression(t *testing.T) {
	source := `
		local rec: {foo: {bar: string}?}? = nil
		local value = rec and rec.foo and rec.foo.bar or "x"
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested and/or expression guard, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_StringFindSecondReturnNarrowedByFirst(t *testing.T) {
	source := `
		function extract_host(url: string): string
			local start, finish = string.find(url, "://")
			if start then
				return url:sub(finish + 1)
			end
			return url
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for string.find co-correlation, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_ModuleExportPreservesParamTypes(t *testing.T) {
	registryMod := testutil.CheckAndExport(`
		local M = {}
		function M.get(id: string): string
			return id
		end
		return M
	`, "registry", testutil.WithStdlib())

	if registryMod.HasError() {
		t.Fatal("provider errors")
	}

	source := `
local registry = require("registry")
local function handler(identifier: string)
	local result = registry.get(identifier)
end
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("registry", registryMod))
	if result.HasError() {
		t.Errorf("typed param should satisfy module function; got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_TableIndexAfterInitGuard(t *testing.T) {
	source := `
		function test(data: {[string]: string}, key: string): integer
			local start, finish = string.find(data[key] or "", "%d+")
			if not start then
				return 0
			end
			return finish - start + 1
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for table index after init guard, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
