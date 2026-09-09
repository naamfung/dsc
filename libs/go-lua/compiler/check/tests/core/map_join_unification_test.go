package core

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Map Join Unification Tests
//
// These tests verify that map types are properly unified at CFG join points
// instead of creating unions like {} | {[K]:V1} | {[K]:V2}.
//
// The desired semantics:
//   {} + {[K]:V}     → {[K]:V}
//   {[K]:V1} + {[K]:V2} → {[K]: V1|V2}
//   {[K]:V} + {[K]:nil} → {[K]: V?}

// TestMapJoin_TwoMapsWithDifferentValues tests joining two maps with the same
// key type but different value types. Should produce {[K]: V1|V2}, not a union.
func TestMapJoin_TwoMapsWithDifferentValues(t *testing.T) {
	source := `
		local function f(flag: boolean)
			local t = {}
			if flag then
				t["key"] = 42
			else
				t["key"] = "hello"
			end
			-- At join: t should be {[string]: number|string}
			-- not {} | {[string]: number} | {[string]: string}
			local v = t["key"]
			return v
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())

	// This test passes if we can index the unified map type
	if result.HasError() {
		t.Errorf("expected no errors for two maps with different values, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestMapJoin_MapValueAndNil tests joining a map with values and a map with nil.
// Should produce {[K]: V?}, not a union.
func TestMapJoin_MapValueAndNil(t *testing.T) {
	source := `
		local function f(flag: boolean)
			local t = {}
			if flag then
				t["key"] = {value = 1}
			else
				t["key"] = nil
			end
			-- At join: t should be {[string]: {value: number}?}
			local v = t["key"]
			if v then
				local n: number = v.value
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors for map value and nil join, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestMapJoin_LoopBuildWithClear tests the common pattern of building a map in
// a loop where entries can be added or cleared. The joined type should be
// {[K]: V?} not a union of multiple map types.
func TestMapJoin_LoopBuildWithClear(t *testing.T) {
	source := `
		local function process_events(events: {type: string, pid: string, from: string?}[])
			local pending = {}

			for _, event in ipairs(events) do
				if event.type == "add" and event.from then
					pending[event.pid] = {from = event.from}
				elseif event.type == "remove" then
					pending[event.pid] = nil
				end
			end

			-- pending should be {[string]: {from: string}?}
			-- Indexing should return {from: string}?
			local op = pending["p1"]
			if op then
				local f: string = op.from
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors for loop build with clear, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestMapJoin_WhileLoopServicePattern tests the exact pattern from the
// spawn_monitored_service issue where pending[event.from] fails due to
// the pending type being a union rather than a unified map.
func TestMapJoin_WhileLoopServicePattern(t *testing.T) {
	source := `
		type Operation = {from: string, request_id: number}

		local function spawn_monitored_service()
			local pending = {}
			local running = true
			local counter = 0

			while running do
				counter = counter + 1
				local pid = "worker_" .. tostring(counter)

				if counter % 3 == 0 then
					-- Add operation
					pending[pid] = {
						from = "caller",
						request_id = counter
					}
				elseif counter % 3 == 1 then
					-- Process and clear operation
					local operation = pending[pid]
					if operation then
						pending[pid] = nil
						-- This is the line that fails if pending is a union:
						local rid: number = operation.request_id
					end
				end

				if counter > 10 then
					running = false
				end
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors for while loop service pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestMapJoin_UnionNotExpected verifies that when map unification works,
// the resulting type is not reported as a union.
func TestMapJoin_UnionNotExpected(t *testing.T) {
	source := `
		local function f(flag: boolean)
			local t = {}
			if flag then
				t["a"] = 1
			else
				t["b"] = 2
			end
			return t
		end

		local result = f(true)
		local v = result["a"]  -- Should work, v is number?
	`
	result := testutil.Check(source, testutil.WithStdlib())

	// Check that we don't have errors mentioning "union"
	for _, d := range result.Diagnostics {
		if strings.Contains(strings.ToLower(d.Message), "union") {
			t.Errorf("unexpected union type in diagnostic: %s", d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestMapJoin_EmptyRecordAndMapOfNil tests the edge case of joining
// {} with {[K]: nil}. When both branches assign via variable key,
// the result should be {[K]: V?}.
func TestMapJoin_EmptyRecordAndMapOfNil(t *testing.T) {
	// Use variable key to trigger indexer widening (not field assignment)
	source := `
		local function f(flag: boolean, key: string)
			local t = {}
			if flag then
				t[key] = {value = 1}  -- widening via indexer
			else
				t[key] = nil  -- widening via indexer
			end
			-- At join: should be {[string]: {value: number}?}
			local v = t["any"]
			if v then
				local n: number = v.value
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors for map with value and nil branches, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestMapJoin_NestedMapUnification tests that nested maps are also unified.
func TestMapJoin_NestedMapUnification(t *testing.T) {
	source := `
		local function f(flag: boolean)
			local t = {}
			if flag then
				t["outer"] = {inner = 1}
			else
				t["outer"] = {inner = 2}
			end
			-- Should unify to {[string]: {inner: number}}
			local v = t["outer"]
			if v then
				local n: number = v.inner
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		t.Errorf("expected no errors for nested map unification, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
