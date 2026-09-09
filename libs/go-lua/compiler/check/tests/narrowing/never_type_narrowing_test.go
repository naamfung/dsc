package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestNeverTypeNarrowing tests patterns that incorrectly produce `never` type,
// causing E0002 "expected function, got never" errors.
//
// These patterns were found in wippy linter output where 34 of 39 errors
// were `never` type propagation bugs.
//
// BUG PATTERNS:
// 1. Sequential field equality guards: `if x.a ~= val then return end; if x.b ~= val2 then return end`
// 2. Second channel.select branch check after first is eliminated
// 3. type() guard stacking after nil check
// 4. Multiple sequential guards on same variable
func TestNeverTypeNarrowing_SequentialFieldGuards(t *testing.T) {
	// Event type with multiple fields
	eventType := typ.NewRecord().
		Field("kind", typ.String).
		Field("from", typ.String).
		Field("result", typ.Any).
		Field("error", typ.NewOptional(typ.String)).
		Build()

	eventsManifest := io.NewManifest("events")
	eventsManifest.SetExport(typ.NewRecord().
		Field("get", typ.Func().Returns(eventType).Build()).
		Build())
	eventsManifest.DefineType("Event", eventType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		// Baseline: single field check
		{
			name: "single_field_check",
			code: `
local function get(): Event
    return events.get()
end

local event = get()
if event.kind ~= "EXIT" then
    return false, "wrong kind"
end
local k = event.kind
`,
			wantError: false,
		},

		// Two sequential field checks - this pattern fails in wippy
		{
			name: "two_sequential_field_checks",
			code: `
local function get(): Event
    return events.get()
end

local event = get()
if event.kind ~= "EXIT" then
    return false, "wrong kind"
end
if event.from ~= "worker" then
    return false, "wrong from"
end
local k = event.kind
local f = event.from
`,
			wantError: false,
		},

		// Three sequential field checks
		{
			name: "three_sequential_field_checks",
			code: `
local function get(): Event
    return events.get()
end

local event = get()
if event.kind ~= "EXIT" then
    return false, "wrong kind"
end
if event.from ~= "worker" then
    return false, "wrong from"
end
if event.error then
    return false, "has error"
end
local k = event.kind
`,
			wantError: false,
		},

		// Field check then function call
		{
			name: "field_check_then_call",
			code: `
local function get(): Event
    return events.get()
end

local function process(e: Event): string
    return e.kind
end

local event = get()
if event.kind ~= "EXIT" then
    return false
end
local result = process(event)
`,
			wantError: false,
		},

		// Field check with error() call
		{
			name: "field_check_with_error_call",
			code: `
local function get(): Event
    return events.get()
end

local event = get()
if event.kind ~= "EXIT" then
    error("wrong kind: " .. event.kind)
end
local k = event.kind
`,
			wantError: false,
		},

		// Two field checks with error() calls
		{
			name: "two_field_checks_with_error",
			code: `
local function get(): Event
    return events.get()
end

local event = get()
if event.kind ~= "EXIT" then
    error("wrong kind")
end
if event.from ~= "worker" then
    error("wrong from")
end
local k = event.kind
local f = event.from
`,
			wantError: false,
		},

		// Nested field checks
		{
			name: "nested_field_checks",
			code: `
local function get(): Event
    return events.get()
end

local event = get()
if event.kind == "EXIT" then
    if event.from == "worker" then
        local k = event.kind
        local f = event.from
    end
end
`,
			wantError: false,
		},

		// Mixed equality and inequality
		{
			name: "mixed_equality_inequality",
			code: `
local function get(): Event
    return events.get()
end

local event = get()
if event.kind == "EXIT" then
    if event.from ~= "unknown" then
        local k = event.kind
    end
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("events", eventsManifest))

			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: [%s] %s", d.Position.Line, d.Code.Name(), d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}

// TestNeverTypeNarrowing_TypeGuardStacking tests the pattern where
// nil check + type() guard produces never instead of the narrowed type.
func TestNeverTypeNarrowing_TypeGuardStacking(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		// Baseline: just nil check
		{
			name: "just_nil_check",
			code: `
local function test(x: any)
    if x == nil then
        return
    end
    return x
end
`,
			wantError: false,
		},

		// nil check then type check
		{
			name: "nil_then_type_check",
			code: `
local function test(x: any)
    if x == nil then
        return nil, "nil"
    end
    if type(x) ~= "table" then
        return nil, "not table"
    end
    return x
end
`,
			wantError: false,
		},

		// not x then type check
		{
			name: "not_x_then_type_check",
			code: `
local function test(x: any)
    if not x then
        return nil, "falsy"
    end
    if type(x) ~= "table" then
        return nil, "not table"
    end
    return x
end
`,
			wantError: false,
		},

		// Three stacked guards
		{
			name: "three_stacked_guards",
			code: `
local function test(x: any)
    if x == nil then
        error("nil")
    end
    if type(x) ~= "table" then
        error("not table")
    end
    if x.field == nil then
        error("no field")
    end
    return x.field
end
`,
			wantError: false,
		},

		// The exact assert.error_kind pattern from wippy
		{
			name: "assert_error_kind_pattern",
			code: `
local function error_kind(err: any, expected_kind: string, msg: string?)
    if err == nil then
        error((msg or "error_kind failed") .. ": error is nil")
    end
    if type(err) ~= "table" then
        error((msg or "error_kind failed") .. ": error is not structured")
    end
    if err.kind ~= expected_kind then
        error((msg or "error_kind failed") .. ": expected kind '" .. expected_kind .. "'")
    end
end
`,
			wantError: false,
		},

		// Type check with field access
		{
			name: "type_check_then_field_access",
			code: `
local function test(x: any)
    if x == nil then
        return nil
    end
    if type(x) ~= "table" then
        return nil
    end
    return x.name
end
`,
			wantError: false,
		},

		// Multiple type checks
		{
			name: "multiple_type_checks",
			code: `
local function test(x: any)
    if type(x) == "nil" then
        return "nil"
    end
    if type(x) == "string" then
        return "string: " .. x
    end
    if type(x) == "number" then
        return "number: " .. tostring(x)
    end
    return "other"
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib())

			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: [%s] %s", d.Position.Line, d.Code.Name(), d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}

// TestNeverTypeNarrowing_ChannelSelectBranches tests the pattern where
// checking channel identity in sequential if statements produces never.
func TestNeverTypeNarrowing_ChannelSelectBranches(t *testing.T) {
	// Create channel types for select result simulation
	chanAType := typ.NewRecord().Field("__tag", typ.String).Build()
	chanBType := typ.NewRecord().Field("__tag", typ.String).Build()
	chanCType := typ.NewRecord().Field("__tag", typ.String).Build()

	selectResultType := typ.NewUnion(
		typ.NewRecord().Field("channel", chanAType).Field("value", typ.String).Build(),
		typ.NewRecord().Field("channel", chanBType).Field("value", typ.Number).Build(),
	)

	selectResult3Type := typ.NewUnion(
		typ.NewRecord().Field("channel", chanAType).Field("value", typ.String).Build(),
		typ.NewRecord().Field("channel", chanBType).Field("value", typ.Number).Build(),
		typ.NewRecord().Field("channel", chanCType).Field("value", typ.Boolean).Build(),
	)

	selectManifest := io.NewManifest("sel")
	selectManifest.SetExport(typ.NewRecord().
		Field("wait2", typ.Func().
			Param("a", chanAType).
			Param("b", chanBType).
			Returns(selectResultType).Build()).
		Field("wait3", typ.Func().
			Param("a", chanAType).
			Param("b", chanBType).
			Param("c", chanCType).
			Returns(selectResult3Type).Build()).
		Build())
	selectManifest.DefineType("ChanA", chanAType)
	selectManifest.DefineType("ChanB", chanBType)
	selectManifest.DefineType("ChanC", chanCType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		// Baseline: single channel check
		{
			name: "single_channel_check",
			code: `
local a: ChanA = {__tag = "chanA"}
local b: ChanB = {__tag = "chanB"}
local result = sel.wait2(a, b)
if result.channel == a then
    local v: string = result.value
end
`,
			wantError: false,
		},

		// Check first, return, then access remaining
		{
			name: "check_first_return_access_second",
			code: `
local a: ChanA = {__tag = "chanA"}
local b: ChanB = {__tag = "chanB"}

local function test()
    local result = sel.wait2(a, b)
    if result.channel == a then
        return result.value
    end
    local v: number = result.value
    return v
end
`,
			wantError: false,
		},

		// Two sequential channel checks (the wippy bus_pattern issue)
		{
			name: "two_sequential_channel_checks",
			code: `
local a: ChanA = {__tag = "chanA"}
local b: ChanB = {__tag = "chanB"}

local function test()
    local result = sel.wait2(a, b)
    if result.channel == a then
        return "got a"
    end
    if result.channel == b then
        local v: number = result.value
        return "got b: " .. tostring(v)
    end
    return "unknown"
end
`,
			wantError: false,
		},

		// Three channels, sequential checks
		{
			name: "three_channels_sequential_checks",
			code: `
local a: ChanA = {__tag = "chanA"}
local b: ChanB = {__tag = "chanB"}
local c: ChanC = {__tag = "chanC"}

local function test()
    local result = sel.wait3(a, b, c)
    if result.channel == a then
        return "a"
    end
    if result.channel == b then
        return "b"
    end
    if result.channel == c then
        return "c"
    end
    return "unknown"
end
`,
			wantError: false,
		},

		// Check with inequality
		{
			name: "channel_inequality_check",
			code: `
local a: ChanA = {__tag = "chanA"}
local b: ChanB = {__tag = "chanB"}

local function test()
    local result = sel.wait2(a, b)
    if result.channel ~= a then
        return "not a"
    end
    local v: string = result.value
    return v
end
`,
			wantError: false,
		},

		// Nested channel checks
		{
			name: "nested_channel_checks",
			code: `
local a: ChanA = {__tag = "chanA"}
local b: ChanB = {__tag = "chanB"}
local flag = true

local function test()
    local result = sel.wait2(a, b)
    if result.channel == a then
        if flag then
            local v: string = result.value
            return v
        end
    end
    return "fallback"
end
`,
			wantError: false,
		},

		// Channel check in else branch
		{
			name: "channel_check_else_branch",
			code: `
local a: ChanA = {__tag = "chanA"}
local b: ChanB = {__tag = "chanB"}

local function test()
    local result = sel.wait2(a, b)
    if result.channel == a then
        local v: string = result.value
        return v
    else
        local v: number = result.value
        return tostring(v)
    end
end
`,
			wantError: false,
		},

		// Channel check with method call after
		{
			name: "channel_check_then_method_call",
			code: `
local a: ChanA = {__tag = "chanA"}
local b: ChanB = {__tag = "chanB"}

local function process(s: string): string
    return s
end

local function test()
    local result = sel.wait2(a, b)
    if result.channel == a then
        return process(result.value)
    end
    return "not a"
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("sel", selectManifest))

			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: [%s] %s", d.Position.Line, d.Code.Name(), d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}

// TestNeverTypeNarrowing_ProcessEventPattern tests the exact patterns
// from wippy's process test files that produce never type.
func TestNeverTypeNarrowing_ProcessEventPattern(t *testing.T) {
	// Actor type with meta() method
	metaType := typ.NewRecord().
		Field("role", typ.String).
		Field("department", typ.String).
		Build()

	actorType := typ.NewInterface("Actor", []typ.Method{
		{Name: "id", Type: typ.Func().Returns(typ.String).Build()},
		{Name: "meta", Type: typ.Func().Returns(typ.NewOptional(metaType)).Build()},
	})

	securityManifest := io.NewManifest("security")
	securityManifest.SetExport(typ.NewRecord().
		Field("actor", typ.Func().Returns(typ.NewOptional(actorType)).Build()).
		Build())
	securityManifest.DefineType("Actor", actorType)
	securityManifest.DefineType("Meta", metaType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		// The actor_validator_worker pattern
		{
			name: "actor_validator_pattern",
			code: `
local function main()
    local actor = security.actor()
    if not actor then
        error("actor not found")
    end

    local id = actor:id()
    if id ~= "test_user" then
        error("wrong id: " .. id)
    end

    local meta = actor:meta()
    if not meta then
        error("meta not found")
    end

    if meta.role ~= "admin" then
        error("wrong role: " .. meta.role)
    end

    if meta.department ~= "engineering" then
        error("wrong department: " .. meta.department)
    end

    return true
end
`,
			wantError: false,
		},

		// Simplified version with just two field checks
		{
			name: "two_meta_field_checks",
			code: `
local function main()
    local actor = security.actor()
    if not actor then
        return nil, "no actor"
    end

    local meta = actor:meta()
    if not meta then
        return nil, "no meta"
    end

    if meta.role ~= "admin" then
        return nil, "wrong role"
    end

    if meta.department ~= "engineering" then
        return nil, "wrong department"
    end

    return true
end
`,
			wantError: false,
		},

		// Field check with string concat in error
		{
			name: "field_check_with_concat_error",
			code: `
local function main()
    local actor = security.actor()
    if not actor then
        error("no actor")
    end

    local meta = actor:meta()
    if not meta then
        error("no meta")
    end

    if meta.role ~= "admin" then
        error("role mismatch: expected admin, got " .. tostring(meta.role))
    end

    return meta.department
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("security", securityManifest))

			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: [%s] %s", d.Position.Line, d.Code.Name(), d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}

// TestNeverTypeNarrowing_LoopAndControlFlow tests never propagation
// in loops and complex control flow.
func TestNeverTypeNarrowing_LoopAndControlFlow(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		// While loop with guard
		{
			name: "while_loop_with_guard",
			code: `
local function process(items: {string})
    local i = 1
    while i <= #items do
        local item = items[i]
        if item == nil then
            break
        end
        if item == "" then
            i = i + 1
        else
            return item
        end
        i = i + 1
    end
    return nil
end
`,
			wantError: false,
		},

		// For loop with multiple guards
		{
			name: "for_loop_multiple_guards",
			code: `
local function find(items: {any}, target: string)
    for i = 1, #items do
        local item = items[i]
        if item == nil then
            break
        end
        if type(item) ~= "string" then
            error("not a string")
        end
        if item == target then
            return i
        end
    end
    return -1
end
`,
			wantError: false,
		},

		// Nested loops with guards
		{
			name: "nested_loops_with_guards",
			code: `
local function search(matrix: {{number}}, target: number)
    for i = 1, #matrix do
        local row = matrix[i]
        if row == nil then
            break
        end
        for j = 1, #row do
            local val = row[j]
            if val == nil then
                break
            end
            if val == target then
                return i, j
            end
        end
    end
    return -1, -1
end
`,
			wantError: false,
		},

		// Guard then loop
		{
			name: "guard_then_loop",
			code: `
local function process(data: {value: string}?)
    if data == nil then
        return nil
    end
    if data.value == "" then
        return nil
    end

    local result = ""
    for i = 1, 3 do
        result = result .. data.value
    end
    return result
end
`,
			wantError: false,
		},

		// Multiple returns after guards
		{
			name: "multiple_returns_after_guards",
			code: `
local function classify(x: any)
    if x == nil then
        return "nil"
    end
    if type(x) == "string" then
        if x == "" then
            return "empty string"
        end
        return "string"
    end
    if type(x) == "number" then
        if x == 0 then
            return "zero"
        end
        if x < 0 then
            return "negative"
        end
        return "positive"
    end
    return "other"
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib())

			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: [%s] %s", d.Position.Line, d.Code.Name(), d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}
