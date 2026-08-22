package tables

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Dynamic Table Insertion False Positives
// These tests document E0004 errors caused by dynamic table insertions
// where the type checker doesn't track that fields were added to {}.

func TestDynamicTable_BracketAssignment(t *testing.T) {
	// Minimal reproduction: assign to empty table via bracket notation
	source := `
		local t = {}
		t["greet"] = true
		local v: boolean = t["greet"]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for bracket assignment to empty table, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_BracketAssignmentRead(t *testing.T) {
	// Read after bracket assignment
	source := `
		local t = {}
		t["greet"] = true
		if t["greet"] ~= true then error("expected greet") end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for reading bracket-assigned field, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_LoopInsertion(t *testing.T) {
	// Dynamic insertion in loop - table is widened from {} to {[K]: V}
	source := `
		local names = {}
		for _, s in ipairs({"A","B"}) do
			names[s] = true
		end
		local v: boolean? = names["A"]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for loop insertion, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_LoopInsertionAssert(t *testing.T) {
	// Dynamic insertion in loop with assert - table is widened from {} to {[K]: V}
	source := `
		local names = {}
		for _, s in ipairs({"A","B"}) do
			names[s] = true
		end
		if names["A"] ~= true then error("expected A") end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for loop insertion with assert, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_MixedAssignment(t *testing.T) {
	// Mix of dot and bracket notation
	source := `
		local t: {[string]: boolean} = {}
		t.foo = true
		t["bar"] = true
		local a: boolean? = t.foo
		local b: boolean? = t["bar"]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for mixed assignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_StringKeyMap(t *testing.T) {
	// Explicit map type should work
	source := `
		local method_names: {[string]: string} = {
			greet = "hello",
			farewell = "goodbye"
		}
		local name: string? = method_names["greet"]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for string key map, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_NumericIndex(t *testing.T) {
	// Numeric index on empty table
	source := `
		local arr = {}
		arr[1] = "first"
		arr[2] = "second"
		local v: string? = arr[1]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for numeric index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_SelfFieldAssignmentsAreVisible(t *testing.T) {
	// Class-like pattern: construct table, assign fields, define methods using self._field.
	source := `
		local function create_reader(dataflow_id: string)
			local reader = {}

			reader._dataflow_id = dataflow_id
			reader._node_ids = nil
			reader._data_types = nil

			function reader:with_nodes(...)
				local copy = self
				copy._node_ids = {...}
				return copy
			end

			function reader:_build_query()
				local id = self._dataflow_id
				if self._node_ids then
					local first = self._node_ids[1]
				end
				return id
			end

			return reader
		end

		local r = create_reader("df1")
		r:_build_query()
		r:with_nodes(1, 2, 3)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for self field assignments, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_GlobalTableFieldPreservedAcrossHelper(t *testing.T) {
	source := `
		type Tx = { query: (self: Tx, q: string) -> number }
		local function make_tx(): Tx
			return { query = function(self: Tx, q: string): number return 1 end }
		end

		local test_ctx = {}
		test_ctx.tx = make_tx()

		local function get_tx()
			return test_ctx.tx
		end

		local tx = get_tx()
		local n: number = tx:query("SELECT 1")
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for global table field preserved across helper, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_VariableKey(t *testing.T) {
	// Variable as key
	source := `
		local t: {[string]: integer} = {}
		local key: string = "count"
		t[key] = 42
		local v: integer? = t[key]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for variable key, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_KeyFromFieldExpr(t *testing.T) {
	// Key derived from field expression should infer numeric key type
	source := `
		local counts = {}
		local msg = { producer = 1 }
		counts[msg.producer] = 1
		local v: integer? = counts[1]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for key from field expression, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_GroupBySuitePattern(t *testing.T) {
	// Pattern from wippy runner.lua: suites[suite] = suites[suite] or {}
	source := `
		local function group_by_suite(entries: {id: string, suite: string?}[])
			local suites = {}
			for _, entry in ipairs(entries) do
				local suite = entry.suite
				if suite then
					suites[suite] = suites[suite] or {}
					table.insert(suites[suite], entry)
				end
			end
			return suites
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for group_by_suite pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_TableInsertToEmptyTable(t *testing.T) {
	// Pattern: local keys = {} followed by table.insert(keys, k)
	source := `
		local function sorted_keys(t: {[string]: any})
			local keys = {}
			for k in pairs(t) do
				table.insert(keys, k)
			end
			return keys
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for table.insert pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_NestedTableInsert(t *testing.T) {
	// Pattern: creating nested table and inserting into it
	source := `
		local results: {passed: integer, failed: integer, errors: {id: string, error: string}[]} = {
			passed = 0,
			failed = 0,
			errors = {},
		}
		table.insert(results.errors, {id = "test", error = "failed"})
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested table.insert, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_PendingMapPattern(t *testing.T) {
	// Pattern from spawn_monitored_service: pending[pid] = value, then pending[pid] = nil
	// Should unify to {[string]: {from: string} | nil} not a union of separate map types
	source := `
		local pending = {}

		local function add_pending(pid: string, from: string)
			pending[pid] = { from = from }
		end

		local function remove_pending(pid: string)
			local op = pending[pid]
			pending[pid] = nil
			return op
		end

		add_pending("p1", "caller1")
		local op = remove_pending("p1")
		if op then
			local f = op.from
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for pending map pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_LoopAssignAndClear(t *testing.T) {
	// Pattern: assign in one branch, clear (nil) in another
	source := `
		local cache = {}

		for i = 1, 10 do
			local key = tostring(i)
			if i % 2 == 0 then
				cache[key] = { value = i }
			else
				cache[key] = nil
			end
		end

		local entry = cache["2"]
		if entry then
			local v: integer = entry.value
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for loop assign and clear, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_ServiceLoopPattern(t *testing.T) {
	// Pattern from spawn_monitored_service: loop with select, pending[pid] indexed
	// The key issue is: pending[event_from] where pending has union type
	source := `
		local pending = {}

		local pids = {"p1", "p2", "p3"}
		for _, pid in ipairs(pids) do
			pending[pid] = {
				from = "caller",
				request_id = 1
			}
		end

		-- Later, index with a variable
		local event_from = "p1"
		local operation = pending[event_from]
		if operation then
			pending[event_from] = nil
			local from = operation.from
			local rid = operation.request_id
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for service loop pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_WhileLoopBranchAssign(t *testing.T) {
	// Pattern from spawn_monitored_service: while loop with branching assignments
	// pending gets assigned in one branch, set to nil in another
	// Should unify to {[string]: V | nil} not {} | {[string]: nil} | {[string]: V}
	source := `
		local pending = {}
		local running = true
		local counter = 0

		while running do
			counter = counter + 1
			if counter > 10 then
				running = false
			elseif counter % 2 == 0 then
				-- Assign value
				local pid = tostring(counter)
				pending[pid] = {
					from = "caller",
					request_id = counter
				}
			else
				-- Clear value
				local pid = tostring(counter - 1)
				pending[pid] = nil
			end
		end

		-- Index the map - should work without union type error
		local op = pending["2"]
		if op then
			local f = op.from
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for while loop branch assign, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDynamicTable_WhileTrueServicePattern(t *testing.T) {
	// Exact pattern from spawn_monitored_service with while true and elseif branches
	// pending[pid] assigned in one elseif, pending[pid] = nil in another elseif
	source := `
		local pending = {}
		local inbox_mode = true
		local iteration = 0

		while true do
			iteration = iteration + 1
			if iteration > 10 then
				break
			end

			if inbox_mode then
				-- One branch: assign to pending
				local worker_pid = "worker_" .. tostring(iteration)
				pending[worker_pid] = {
					from = "caller",
					respond_to = "response",
					request_id = iteration
				}
				inbox_mode = false
			else
				-- Other branch: read and clear pending
				local event_from = "worker_" .. tostring(iteration - 1)
				local operation = pending[event_from]
				if operation then
					pending[event_from] = nil
					local rid = operation.request_id
				end
				inbox_mode = true
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for while true service pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestOpenRecord_DefaultEmptyTable(t *testing.T) {
	source := `
		local t = {}
		t.x = 1
		local v = t.x
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for open empty table with field assignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestOpenRecord_FieldPreservation(t *testing.T) {
	source := `
		local t = {}
		t.name = "foo"
		t.age = 10
		local n = t.name
		local a = t.age
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for field preservation on open table, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestOpenRecord_UnknownFieldAccess(t *testing.T) {
	source := `
		local t = {}
		t.x = 1
		local v = t.y
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for unknown field access on open table, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCompositeTable_RecordWithMapComponent(t *testing.T) {
	t.Run("field assignment after dynamic index", func(t *testing.T) {
		source := `
			local t = {}
			local key: string = "k"
			t[key] = 42
			t.foo = 1
			local y: integer = t.foo
		`
		result := testutil.Check(source, testutil.WithStdlib())
		if result.HasError() {
			t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	t.Run("dynamic index on record with fields", func(t *testing.T) {
		source := `
			local t = {}
			t.name = "hello"
			local key: string = "k"
			t[key] = 42
		`
		result := testutil.Check(source, testutil.WithStdlib())
		if result.HasError() {
			t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})
}
