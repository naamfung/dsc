package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestChannelSelectNarrowing_IfReturnPattern tests:
//
//	if result.channel == timeout then return end
//	local event = result.value -- should be narrowed to Event, not Event|Time
func TestChannelSelectNarrowing_IfReturnPattern(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		type Event = { kind: string, result: any? }
		type Time = { sec: number, nsec: number }

		function f(events_ch: Channel<Event>, timeout: Channel<Time>)
			local result = channel.select {
				events_ch:case_receive(),
				timeout:case_receive(),
			}
			if result.channel == timeout then
				return false
			end
			-- After the return, result should be narrowed to only the Event variant
			local event = result.value
			-- event should now be Event, not Event|Time
			-- This MUST work - accessing .kind on narrowed Event type
			local k: string = event.kind
			-- This MUST fail if narrowing doesn't work - Time has no .kind field
			return true
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after channel narrowing, but got %d errors", len(result.Errors))
	}
}

// TestChannelSelectNarrowing_MustFailWithoutNarrowing tests that accessing
// a variant-specific field WITHOUT narrowing produces an error.
func TestChannelSelectNarrowing_MustFailWithoutNarrowing(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		type Event = { kind: string }
		type Time = { sec: number }

		function f(events_ch: Channel<Event>, timeout: Channel<Time>)
			local result = channel.select {
				events_ch:case_receive(),
				timeout:case_receive(),
			}
			-- NO narrowing - accessing .kind should fail because Time has no .kind
			local event = result.value
			local k: string = event.kind
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	// This test SHOULD fail (WantError=true) if the type system is sound
	// Because without narrowing, result.value is Event|Time and .kind doesn't exist on Time
	if !result.HasError() {
		t.Errorf("expected error when accessing .kind without narrowing, but got none")
	}
}

// TestChannelSelectNarrowing_ElseBranchNarrowsToOther tests that the else branch
// is narrowed to the other variant.
func TestChannelSelectNarrowing_ElseBranchNarrowsToOther(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		type ChanInt = {__tag: "int"}
		type ChanStr = {__tag: "str"}
		type SelResult =
			{channel: ChanInt, value: number, ok: boolean} |
			{channel: ChanStr, value: string, ok: boolean}

		function get_result(a: ChanInt, b: ChanStr): SelResult
			return {channel = a, value = 42, ok = true}
		end

		function f(ch1: ChanInt, ch2: ChanStr)
			local result = get_result(ch1, ch2)
			if result.channel == ch1 then
				-- Narrowed to first variant, value is number
				local n: number = result.value
			else
				-- Narrowed to second variant, value is string
				local s: string = result.value
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors with else branch narrowing")
	}
}

// TestChannelSelectNarrowing_ElseBranchWrongType tests that assigning wrong type
// in else branch produces error.
func TestChannelSelectNarrowing_ElseBranchWrongType(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		type ChanInt = {__tag: "int"}
		type ChanStr = {__tag: "str"}
		type SelResult =
			{channel: ChanInt, value: number, ok: boolean} |
			{channel: ChanStr, value: string, ok: boolean}

		function get_result(a: ChanInt, b: ChanStr): SelResult
			return {channel = a, value = 42, ok = true}
		end

		function f(ch1: ChanInt, ch2: ChanStr)
			local result = get_result(ch1, ch2)
			if result.channel == ch1 then
				local n: number = result.value
			else
				-- WRONG: else branch has string value, not number
				local n: number = result.value
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	// Should error because result.value in else branch is string, not number
	if !result.HasError() {
		t.Errorf("expected error when assigning string to number in else branch")
	}
}

// TestChannelSelectNarrowing_NestedFieldAccess tests accessing nested fields after narrowing.
func TestChannelSelectNarrowing_NestedFieldAccess(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		type ChanInt = {__tag: "int"}
		type ChanStr = {__tag: "str"}
		type SelResult =
			{channel: ChanInt, value: {error: string}, ok: boolean} |
			{channel: ChanStr, value: {data: number}, ok: boolean}

		function get_result(a: ChanInt, b: ChanStr): SelResult
			return {channel = a, value = {error = "oops"}, ok = true}
		end

		function f(ch1: ChanInt, ch2: ChanStr)
			local result = get_result(ch1, ch2)
			if result.channel == ch1 then
				-- Narrowed to first variant, value has .error field
				local e: string = result.value.error
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after nested field narrowing")
	}
}

// TestChannelSelectNarrowing_NestedFieldWrongAccess tests that accessing wrong nested field fails.
func TestChannelSelectNarrowing_NestedFieldWrongAccess(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		type ChanInt = {__tag: "int"}
		type ChanStr = {__tag: "str"}
		type SelResult =
			{channel: ChanInt, value: {error: string}, ok: boolean} |
			{channel: ChanStr, value: {data: number}, ok: boolean}

		function get_result(a: ChanInt, b: ChanStr): SelResult
			return {channel = a, value = {error = "oops"}, ok = true}
		end

		function f(ch1: ChanInt, ch2: ChanStr)
			local result = get_result(ch1, ch2)
			if result.channel == ch1 then
				-- WRONG: first variant has .error, not .data
				local d: number = result.value.data
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	// Should error because .data doesn't exist on first variant's value
	if !result.HasError() {
		t.Errorf("expected error when accessing .data on first variant")
	}
}

// TestChannelSelectNarrowing_FunctionReturnTypes tests with channels from function returns.
func TestChannelSelectNarrowing_FunctionReturnTypes(t *testing.T) {
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	eventType := typ.NewRecord().
		Field("kind", typ.String).
		OptField("result", typ.Any).
		Build()

	timeType := typ.NewInterface("Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
	})

	processManifest := testutil.ProcessManifest(channelGeneric, eventType)
	timeManifest := testutil.TimeManifest(channelGeneric, timeType)

	source := `
		function main()
			local events_ch = process.events()
			local timeout = time.after("3s")

			local result = channel.select {
				events_ch:case_receive(),
				timeout:case_receive(),
			}

			if result.channel == timeout then
				return false, "timeout"
			end

			-- After the return, result should be narrowed to Event variant
			local event = result.value
			local k: string = event.kind
			local r = event.result
			return true
		end
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
		testutil.WithManifest("time", timeManifest),
	)

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors with function return channel types")
	}
}

// TestContractSpecNarrowing tests that contract specs narrow return types
// based on argument values (e.g., process.listen with message option).
func TestContractSpecNarrowing(t *testing.T) {
	// Create a Message type
	messageType := typ.NewRecord().
		Field("topic", typ.String).
		Field("payload", typ.Any).
		Field("from", typ.Func().Returns(typ.String).Build()).
		Build()

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	// Create contract spec for listen: when opts.message == true, return Channel<Message>
	listenSpec := contract.NewSpec().
		WithReturnCase(
			constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
				Target: constraint.Path{Root: "$1"},
				Field:  "message",
				Value:  typ.LiteralBool(true),
			})),
			typ.Instantiate(channelGeneric, messageType),
		).
		WithDefaultReturn(typ.Instantiate(channelGeneric, typ.Any))

	// Create process module with listen function that should narrow based on opts
	processModule := typ.NewRecord().
		Field("listen", typ.Func().
			Param("topic", typ.String).
			OptParam("opts", typ.NewRecord().OptField("message", typ.Boolean).Build()).
			Returns(typ.Instantiate(channelGeneric, typ.Any)).
			Spec(listenSpec).
			Build()).
		Field("send", typ.Func().
			Param("target", typ.String).
			Param("payload", typ.Any).
			Build()).
		Build()

	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "listen_with_message_option_narrows_to_Message",
			code: `
				local ch = process.listen("increment", {message = true})
				local msg, ok = ch:receive()
				local reply_to: string = msg:from()
				process.send(reply_to, "ack")
			`,
			wantError: false,
		},
		{
			name: "listen_without_option_returns_any",
			code: `
				local ch = process.listen("events")
				local val, ok = ch:receive()
				local x = val
			`,
			wantError: false,
		},
		{
			name: "message_method_call_after_receive",
			code: `
				local ch = process.listen("requests", {message = true})
				local msg, ok = ch:receive()
				if ok then
					local sender: string = msg:from()
					local topic: string = msg.topic
				end
			`,
			wantError: false,
		},
		{
			name: "listen_message_option_produces_string_reply_to",
			code: `
				local ch = process.listen("topic", {message = true})
				local msg, ok = ch:receive()
				local reply_to: string = msg:from()
			`,
			wantError: false,
		},
		{
			name: "spec_narrowing_propagates_through_conditional",
			code: `
				local ch = process.listen("topic", {message = true})
				local msg, ok = ch:receive()
				if ok then
					local s: string = msg:from()
				end
			`,
			wantError: false,
		},
		{
			name: "spec_narrowing_survives_post_assignment_call",
			code: `
				local ch = process.listen("topic", {message = true})
				local msg, ok = ch:receive()
				local reply = msg:from()
				process.send(reply, "ack")
			`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest), testutil.WithManifest("process", processManifest))
			if result.HasError() != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// TestChannelSelectNarrowing_ThreeCasesWithUntyped tests select with two typed channels
// and one untyped channel. The union should include all three variants and narrowing
// should work for the typed ones.
func TestChannelSelectNarrowing_ThreeCasesWithUntyped(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		type Event = { kind: string, data: any? }
		type Time = { sec: number, nsec: number }

		function f(events_ch: Channel<Event>, timeout: Channel<Time>, untyped_ch: Channel<any>)
			local result = channel.select {
				events_ch:case_receive(),
				timeout:case_receive(),
				untyped_ch:case_receive(),
			}

			-- Without narrowing, result.value is Event|Time|any
			-- After narrowing on timeout, should be Event|any (not just Event)
			if result.channel == timeout then
				-- Narrowed to Time variant
				local t: Time = result.value
				return "timeout", t.sec
			end

			-- After eliminating timeout, result.value is Event|any
			if result.channel == events_ch then
				-- Narrowed to Event variant
				local ev: Event = result.value
				return "event", ev.kind
			end

			-- Else branch: must be untyped variant
			local x = result.value
			return "untyped", x
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors with 3-case select (2 typed + 1 untyped)")
	}
}

// TestChannelSelectNarrowing_UntypedCasePreservesTypedVariants tests that when an untyped
// channel is in the select, the typed variants are still preserved in the union and can
// be narrowed.
func TestChannelSelectNarrowing_UntypedCasePreservesTypedVariants(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		type Event = { kind: string }

		function f(events_ch: Channel<Event>, untyped_ch: Channel<any>)
			local result = channel.select {
				events_ch:case_receive(),
				untyped_ch:case_receive(),
			}

			if result.channel == events_ch then
				-- After narrowing, result.value should be Event (not any)
				local ev = result.value
				local k: string = ev.kind
				return k
			end

			-- Else: untyped variant
			return result.value
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors when untyped case preserves typed variants")
	}
}

// TestContractSpecNarrowing_WippyPattern tests with the exact pattern used in wippy:
// - Interface type with methods
// - constraint.ParamPath(1) instead of Path{Root: "$1"}
// - typ.True instead of typ.LiteralBool(true)
func TestContractSpecNarrowing_WippyPattern(t *testing.T) {
	// Create a Message type as Interface (like wippy)
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "from", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	messageChannelType := typ.Instantiate(channelGeneric, messageType)
	rawChannelType := typ.Instantiate(channelGeneric, typ.Any)

	// Exact pattern from wippy's process.listen
	listenSpec := contract.NewSpec().WithReturnCase(
		constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
			Target: constraint.ParamPath(1),
			Field:  "message",
			Value:  typ.True,
		})),
		messageChannelType,
	)

	// Process module interface like wippy
	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "listen", Type: typ.Func().
			Param("topic", typ.String).
			OptParam("options", typ.Any).
			Returns(rawChannelType, typ.NewOptional(typ.LuaError)).
			Spec(listenSpec).
			Build()},
		{Name: "send", Type: typ.Func().
			Param("pid", typ.String).
			Param("topic", typ.String).
			Variadic(typ.Any).
			Returns(typ.Boolean, typ.NewOptional(typ.String)).
			Build()},
	})

	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "listen_with_message_narrowed_to_interface",
			code: `
				local ch = process.listen("increment", {message = true})
				local msg, ok = ch:receive()
				local sender: string = msg:from()
			`,
			wantError: false,
		},
		{
			name: "receive_from_narrowed_channel",
			code: `
				local ch = process.listen("requests", {message = true})
				local msg, ok = ch:receive()
				if ok then
					local from: string = msg:from()
					local topic: string = msg:topic()
					local payload = msg:payload()
				end
			`,
			wantError: false,
		},
		{
			name: "method_call_after_receive_then_send",
			code: `
				local ch = process.listen("requests", {message = true})
				local msg, ok = ch:receive()
				local sender = msg:from()
				process.send(sender, "ack", "data")
			`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest), testutil.WithManifest("process", processManifest))
			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: %s", d.Position.Line, d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// TestSpecNarrowingMethodChainPropagation tests that spec narrowing propagates
// through method call chains like msg:from() → reply_to → process.send(reply_to, ...).
// This reproduces the remaining reply_to errors from wippy lint.
func TestSpecNarrowingMethodChainPropagation(t *testing.T) {
	// Create a Message type as Interface (like wippy)
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "from", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	messageChannelType := typ.Instantiate(channelGeneric, messageType)
	rawChannelType := typ.Instantiate(channelGeneric, typ.Any)

	// Exact pattern from wippy's process.listen
	listenSpec := contract.NewSpec().WithReturnCase(
		constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
			Target: constraint.ParamPath(1),
			Field:  "message",
			Value:  typ.True,
		})),
		messageChannelType,
	)

	// Process module interface like wippy
	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "listen", Type: typ.Func().
			Param("topic", typ.String).
			OptParam("options", typ.Any).
			Returns(rawChannelType, typ.NewOptional(typ.LuaError)).
			Spec(listenSpec).
			Build()},
		{Name: "send", Type: typ.Func().
			Param("pid", typ.String).
			Param("topic", typ.String).
			Variadic(typ.Any).
			Returns(typ.Boolean, typ.NewOptional(typ.String)).
			Build()},
	})

	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "basic_chain_msg_from_to_send",
			code: `
				local ch = process.listen("topic", {message = true})
				local msg = ch:receive()
				local reply = msg:from()
				process.send(reply, "ack")
			`,
			wantError: false,
		},
		{
			name: "multi_assign_chain",
			code: `
				local ch = process.listen("topic", {message = true})
				local msg, ok = ch:receive()
				local reply = msg:from()
				process.send(reply, "ack")
			`,
			wantError: false,
		},
		{
			name: "conditional_chain",
			code: `
				local ch = process.listen("topic", {message = true})
				local msg, ok = ch:receive()
				if ok then
					local reply = msg:from()
					process.send(reply, "ack")
				end
			`,
			wantError: false,
		},
		{
			name: "chained_method_calls_inline",
			code: `
				local ch = process.listen("topic", {message = true})
				local msg, ok = ch:receive()
				process.send(msg:from(), "ack")
			`,
			wantError: false,
		},
		{
			name: "multiple_method_calls_same_receiver",
			code: `
				local ch = process.listen("topic", {message = true})
				local msg, ok = ch:receive()
				local sender: string = msg:from()
				local topic_str: string = msg:topic()
				process.send(sender, topic_str)
			`,
			wantError: false,
		},
		{
			name: "loop_with_break_pattern",
			code: `
				local done = false
				local ch = process.listen("topic", {message = true})
				while not done do
					local msg, ok = ch:receive()
					if not ok then break end
					local reply_to = msg:from()
					process.send(reply_to, "ack")
				end
			`,
			wantError: false,
		},
		{
			name: "loop_with_conditional_after_break",
			code: `
				local done = false
				local ch = process.listen("topic", {message = true})
				while not done do
					local msg, ok = ch:receive()
					if not ok then break end
					local reply_to = msg:from()
					local data = msg:payload()
					if data then
						process.send(reply_to, "nak", "error")
					else
						process.send(reply_to, "ack")
					end
				end
			`,
			wantError: false,
		},
		{
			name: "coroutine_spawn_with_loop",
			code: `
				local done = false
				local function worker()
					local ch = process.listen("increment", {message = true})
					while not done do
						local msg, ok = ch:receive()
						if not ok then break end
						local reply_to = msg:from()
						process.send(reply_to, "ack")
					end
				end
				worker()
			`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest), testutil.WithManifest("process", processManifest))
			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: %s", d.Position.Line, d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// TestSpecNarrowingWithNestedFunctionAndTypeCheck reproduces the bug where
// spec narrowing breaks when:
// 1. Outer function has a parameter
// 2. Inner function in coroutine.spawn
// 3. type() check on nested field (e.g., type(data.amount) ~= "number")
// This combination causes msg:from() to return `any` instead of `string`.
func TestSpecNarrowingWithNestedFunctionAndTypeCheck(t *testing.T) {
	// Create Message interface
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "from", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	messageChannelType := typ.Instantiate(channelGeneric, messageType)
	rawChannelType := typ.Instantiate(channelGeneric, typ.Any)

	// Spec for listen
	listenSpec := contract.NewSpec().WithReturnCase(
		constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
			Target: constraint.ParamPath(1),
			Field:  "message",
			Value:  typ.True,
		})),
		messageChannelType,
	)

	// Process module
	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "listen", Type: typ.Func().
			Param("topic", typ.String).
			OptParam("options", typ.Any).
			Returns(rawChannelType, typ.NewOptional(typ.LuaError)).
			Spec(listenSpec).
			Build()},
		{Name: "send", Type: typ.Func().
			Param("pid", typ.String).
			Param("topic", typ.String).
			Variadic(typ.Any).
			Returns(typ.Boolean, typ.NewOptional(typ.String)).
			Build()},
	})

	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			// MINIMAL FAIL: or condition with any two type checks
			name: "or_with_any_two_type_checks",
			code: `
local ch = process.listen("topic", {message = true})
local msg = ch:receive()
local reply_to = msg:from()
local x = 1
if type(x) ~= "number" or type(x) ~= "number" then
    process.send(reply_to, "nak")
end
`,
			wantError: false,
		},
		{
			// Or with single type check - should PASS
			name: "or_with_single_type_check",
			code: `
local ch = process.listen("topic", {message = true})
local msg = ch:receive()
local reply_to = msg:from()
local x = 1
if type(x) ~= "number" then
    process.send(reply_to, "nak")
end
`,
			wantError: false,
		},
		{
			// Or with just `or false` - minimal or
			name: "or_with_false",
			code: `
local ch = process.listen("topic", {message = true})
local msg = ch:receive()
local reply_to = msg:from()
if type(1) ~= "number" or false then
    process.send(reply_to, "nak")
end
`,
			wantError: false,
		},
		{
			// Or with two unrelated conditions
			name: "or_with_two_unrelated",
			code: `
local ch = process.listen("topic", {message = true})
local msg = ch:receive()
local reply_to = msg:from()
local a = 1
local b = 2
if a == 0 or b == 0 then
    process.send(reply_to, "nak")
end
`,
			wantError: false,
		},
		{
			// Without any or condition
			name: "no_or_condition",
			code: `
local ch = process.listen("topic", {message = true})
local msg = ch:receive()
local reply_to = msg:from()
process.send(reply_to, "nak")
`,
			wantError: false,
		},
		{
			// Or with method call
			name: "or_with_method_call_in_condition",
			code: `
local ch = process.listen("topic", {message = true})
local msg = ch:receive()
local reply_to = msg:from()
if msg:payload() == nil or false then
    process.send(reply_to, "nak")
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest), testutil.WithManifest("process", processManifest))
			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: %s", d.Position.Line, d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// TestChannelSelectNarrowing_NeverAfterExclude reproduces the wippy false positive:
// After excluding timeout variant via `if result.channel == timeout then error() end`,
// the remaining result.value becomes `never` instead of narrowing to Message type.
//
// This is the exact pattern from wippy's multi_topic_worker.lua that fails with:
// "E0002: expected function, got never" on msg:topic()
func TestChannelSelectNarrowing_NeverAfterExclude(t *testing.T) {
	// Create Message interface (like wippy's process.Message)
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})

	// Create Time type
	timeType := typ.NewRecord().
		Field("sec", typ.Number).
		Field("nsec", typ.Number).
		Build()

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	messageChannelType := typ.Instantiate(channelGeneric, messageType)
	timeChannelType := typ.Instantiate(channelGeneric, timeType)

	// Process module with listen that returns Channel<Message> via spec
	listenSpec := contract.NewSpec().WithReturnCase(
		constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
			Target: constraint.ParamPath(1),
			Field:  "message",
			Value:  typ.True,
		})),
		messageChannelType,
	)

	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "listen", Type: typ.Func().
			Param("topic", typ.String).
			OptParam("options", typ.Any).
			Returns(typ.Instantiate(channelGeneric, typ.Any)).
			Spec(listenSpec).
			Build()},
	})

	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	// Time module with after
	timeModule := typ.NewInterface("time", []typ.Method{
		{Name: "after", Type: typ.Func().
			Param("duration", typ.String).
			Returns(timeChannelType).
			Build()},
	})

	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(timeModule)
	timeManifest.DefineType("Time", timeType)

	// The exact pattern from wippy's multi_topic_worker.lua
	source := `
		local ch_a = process.listen("topic_a", {message = true})
		local timeout = time.after("3s")

		local result = channel.select {
			ch_a:case_receive(),
			timeout:case_receive(),
		}

		if result.channel == timeout then
			error("timeout")
		end

		-- After excluding timeout, result.value should be Message, not never
		local msg = result.value
		local topic = msg:topic()  -- BUG: E0002 expected function, got never
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
		testutil.WithManifest("time", timeManifest),
	)

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after channel narrowing, but got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestChannelSelectNarrowing_InboxAndTimeout reproduces the wippy error:
// E0022: no method topic (exists on process.Message, missing on time.Time)
// The pattern is inbox_ch + timeout select, then `if result.channel == timeout then return end`
// After the return, msg:topic() should work but currently fails.
//
// Key finding: Time is an INTERFACE type. When both Message and Time are interfaces,
// the narrowing fails because the channel identity comparison doesn't properly exclude
// the Time variant from the union.
func TestChannelSelectNarrowing_InboxAndTimeout(t *testing.T) {
	// Create Message interface with topic method (from inbox)
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})

	// Create Time type (from timeout) - INTERFACE type like wippy's time.Time
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
	})

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	messageChannelType := typ.Instantiate(channelGeneric, messageType)
	timeChannelType := typ.Instantiate(channelGeneric, timeType)

	// Process module with inbox() returning Channel<Message>
	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "inbox", Type: typ.Func().Returns(messageChannelType).Build()},
	})
	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	// Time module with after() returning Channel<Time>
	timeModule := typ.NewInterface("time", []typ.Method{
		{Name: "after", Type: typ.Func().Param("duration", typ.String).Returns(timeChannelType).Build()},
	})
	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(timeModule)
	timeManifest.DefineType("Time", timeType)

	// Exact pattern from wippy's link_explicit.lua lines 38-50
	source := `
		local inbox_ch = process.inbox()
		local timeout = time.after("2s")
		local result = channel.select {
			inbox_ch:case_receive(),
			timeout:case_receive(),
		}

		if result.channel == timeout then
			return false, "timeout waiting for link confirmation"
		end

		local msg = result.value
		local topic = msg:topic()  -- E0022: no method topic (missing on time.Time)
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
		testutil.WithManifest("time", timeManifest),
	)

	t.Log("=== Diagnostic Analysis ===")
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("ERROR at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after channel narrowing for msg:topic() with inbox+timeout select")
	}
}

// TestChannelSelectNarrowing_EventsAndTimeout tests Event (Record) + Time (Interface).
// This configuration PASSES, demonstrating that Record+Interface works but Interface+Interface fails.
func TestChannelSelectNarrowing_EventsAndTimeout(t *testing.T) {
	// Create Event type with kind field (from events channel) - RECORD type
	eventType := typ.NewRecord().
		Field("kind", typ.String).
		OptField("from", typ.String).
		OptField("result", typ.Any).
		Build()

	// Create Time type (from timeout) - INTERFACE type
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
	})

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	eventChannelType := typ.Instantiate(channelGeneric, eventType)
	timeChannelType := typ.Instantiate(channelGeneric, timeType)

	// Process module with events() returning Channel<Event>
	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "events", Type: typ.Func().Returns(eventChannelType).Build()},
	})
	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Event", eventType)

	// Time module with after() returning Channel<Time>
	timeModule := typ.NewInterface("time", []typ.Method{
		{Name: "after", Type: typ.Func().Param("duration", typ.String).Returns(timeChannelType).Build()},
	})
	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(timeModule)
	timeManifest.DefineType("Time", timeType)

	// Exact pattern from wippy's link_explicit.lua lines 65-76
	source := `
		local events_ch = process.events()
		local timeout = time.after("3s")

		local result = channel.select {
			events_ch:case_receive(),
			timeout:case_receive(),
		}

		if result.channel == timeout then
			return false, "timeout waiting for exit"
		end

		local event = result.value
		local k = event.kind  -- E0004: field 'kind' does not exist on time.Time
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
		testutil.WithManifest("time", timeManifest),
	)

	t.Log("=== Diagnostic Analysis ===")
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("ERROR at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after channel narrowing for event.kind with events+timeout select")
	}
}

// TestChannelSelectNarrowing_EventsAndTimeoutBothInterface tests Event (Interface) + Time (Interface).
// This configuration is expected to FAIL, matching the InboxAndTimeout failure.
func TestChannelSelectNarrowing_EventsAndTimeoutBothInterface(t *testing.T) {
	// Create Event type as INTERFACE (with method instead of field)
	eventType := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "from", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})

	// Create Time type as INTERFACE
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
	})

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	eventChannelType := typ.Instantiate(channelGeneric, eventType)
	timeChannelType := typ.Instantiate(channelGeneric, timeType)

	// Process module with events() returning Channel<Event>
	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "events", Type: typ.Func().Returns(eventChannelType).Build()},
	})
	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Event", eventType)

	// Time module with after() returning Channel<Time>
	timeModule := typ.NewInterface("time", []typ.Method{
		{Name: "after", Type: typ.Func().Param("duration", typ.String).Returns(timeChannelType).Build()},
	})
	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(timeModule)
	timeManifest.DefineType("Time", timeType)

	source := `
		local events_ch = process.events()
		local timeout = time.after("3s")

		local result = channel.select {
			events_ch:case_receive(),
			timeout:case_receive(),
		}

		if result.channel == timeout then
			return false, "timeout"
		end

		local event = result.value
		local k = event:kind()  -- Method call on Interface type
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
		testutil.WithManifest("time", timeManifest),
	)

	t.Log("=== Diagnostic Analysis (Both Interface) ===")
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("ERROR at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors with both types as Interface (but currently fails)")
	}
}

// TestChannelSelectNarrowing_EventKindAfterSelect reproduces issue:
// event.kind missing on time.Time after channel.select exclusion.
// After `if result.channel == timeout then return end`, the remaining
// result.value should be narrowed to Event type, allowing access to .kind field.
func TestChannelSelectNarrowing_EventKindAfterSelect(t *testing.T) {
	// Create Event type with kind field
	eventType := typ.NewRecord().
		Field("kind", typ.String).
		OptField("result", typ.Any).
		Build()

	// Create Time type without kind field
	timeType := typ.NewRecord().
		Field("sec", typ.Number).
		Field("nsec", typ.Number).
		Build()

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	eventChannelType := typ.Instantiate(channelGeneric, eventType)
	timeChannelType := typ.Instantiate(channelGeneric, timeType)

	// Process module with events() returning Channel<Event>
	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "events", Type: typ.Func().Returns(eventChannelType).Build()},
	})
	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Event", eventType)

	// Time module with after() returning Channel<Time>
	timeModule := typ.NewInterface("time", []typ.Method{
		{Name: "after", Type: typ.Func().Param("duration", typ.String).Returns(timeChannelType).Build()},
	})
	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(timeModule)
	timeManifest.DefineType("Time", timeType)

	source := `
		local events_ch = process.events()  -- Channel<process.Event>
		local timeout = time.after("5s")    -- Channel<time.Time>

		local result = channel.select{
			events_ch:case_receive(),
			timeout:case_receive(),
		}

		if result.channel == timeout then return end
		local event = result.value
		local k = event.kind  -- should be string, currently fails
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
		testutil.WithManifest("time", timeManifest),
	)

	t.Log("=== Diagnostic Analysis ===")
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("ERROR at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after channel narrowing for event.kind access")
	}
}

// TestChannelSelectNarrowing_ThreeChannelsTwoSameType reproduces the exact
// pattern from wippy's multi_topic_worker.lua:
// - Three channels in select: ch_a, ch_b (both Channel<Message>), and timeout (Channel<Time>)
// - After excluding timeout, result.value should be Message (not never)
func TestChannelSelectNarrowing_ThreeChannelsTwoSameType(t *testing.T) {
	// Create Message interface with topic and payload methods
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})

	// Create Time type
	timeType := typ.NewRecord().
		Field("sec", typ.Number).
		Field("nsec", typ.Number).
		Build()

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	messageChannelType := typ.Instantiate(channelGeneric, messageType)
	rawChannelType := typ.Instantiate(channelGeneric, typ.Any)
	timeChannelType := typ.Instantiate(channelGeneric, timeType)

	// Process module with listen that returns Channel<Message> via spec
	listenSpec := contract.NewSpec().WithReturnCase(
		constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
			Target: constraint.ParamPath(1),
			Field:  "message",
			Value:  typ.True,
		})),
		messageChannelType,
	)

	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "listen", Type: typ.Func().
			Param("topic", typ.String).
			OptParam("options", typ.Any).
			Returns(rawChannelType).
			Spec(listenSpec).
			Build()},
	})
	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	// Time module with after() returning Channel<Time>
	timeModule := typ.NewInterface("time", []typ.Method{
		{Name: "after", Type: typ.Func().Param("duration", typ.String).Returns(timeChannelType).Build()},
	})
	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(timeModule)
	timeManifest.DefineType("Time", timeType)

	// Exact pattern from multi_topic_worker.lua
	source := `
		local ch_a = process.listen("topic_a", { message = true })
		local ch_b = process.listen("topic_b", { message = true })
		local timeout = time.after("3s")

		local result = channel.select {
			ch_a:case_receive(),
			ch_b:case_receive(),
			timeout:case_receive(),
		}

		if result.channel == timeout then
			error("timeout")
		end

		local msg = result.value
		local topic = msg:topic()   -- BUG: E0002 expected function, got never
		local payload = msg:payload()
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
		testutil.WithManifest("time", timeManifest),
	)

	t.Log("=== Diagnostic Analysis ===")
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("ERROR at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after channel narrowing for msg:topic() with 3-channel select")
	}
}

// TestChannelSelectNarrowing_MsgTopicAfterExclusion reproduces issue:
// msg:topic() becomes never after channel.select exclusion.
// After `if result.channel == timeout then error("timeout") end`,
// the remaining result.value should be narrowed to Message type.
func TestChannelSelectNarrowing_MsgTopicAfterExclusion(t *testing.T) {
	// Create Message interface with topic method
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})

	// Create Time type
	timeType := typ.NewRecord().
		Field("sec", typ.Number).
		Field("nsec", typ.Number).
		Build()

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	messageChannelType := typ.Instantiate(channelGeneric, messageType)
	rawChannelType := typ.Instantiate(channelGeneric, typ.Any)
	timeChannelType := typ.Instantiate(channelGeneric, timeType)

	// Process module with listen that returns Channel<Message> via spec
	listenSpec := contract.NewSpec().WithReturnCase(
		constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
			Target: constraint.ParamPath(1),
			Field:  "message",
			Value:  typ.True,
		})),
		messageChannelType,
	)

	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "listen", Type: typ.Func().
			Param("topic", typ.String).
			OptParam("options", typ.Any).
			Returns(rawChannelType).
			Spec(listenSpec).
			Build()},
	})
	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	// Time module with after() returning Channel<Time>
	timeModule := typ.NewInterface("time", []typ.Method{
		{Name: "after", Type: typ.Func().Param("duration", typ.String).Returns(timeChannelType).Build()},
	})
	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(timeModule)
	timeManifest.DefineType("Time", timeType)

	source := `
		local ch = process.listen("topic", {message = true})
		local timeout = time.after("3s")
		local result = channel.select{ ch:case_receive(), timeout:case_receive() }
		if result.channel == timeout then error("timeout") end
		local msg = result.value
		local topic = msg:topic() -- should pass, currently "never"
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
		testutil.WithManifest("time", timeManifest),
	)

	t.Log("=== Diagnostic Analysis ===")
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("ERROR at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after channel narrowing for msg:topic() access")
	}
}

// TestChannelSelectNarrowing_ChannelVariableNotNarrowed reproduces the wippy false positive:
// After `if result.channel == stop_signal then return end`, the variable `stop_signal`
// itself should NOT be narrowed to never. Only `result.channel` should be narrowed.
// The channel variable is a separate symbol from the select result field.
//
// Pattern from wippy's bus_pattern.lua:
//
//	local stop_signal = channel.new(0)
//	local ops_channel = channel.new(256)
//	local result = channel.select{stop_signal:case_receive(), ops_channel:case_receive()}
//	if result.channel == stop_signal then return end
//	if result.channel == ops_channel then
//	    stop_signal:send(true)  -- BUG: E0002 expected function, got never
//	end
func TestChannelSelectNarrowing_ChannelVariableNotNarrowed(t *testing.T) {
	// Create channel types with send method
	boolChanType := typ.NewInterface("BoolChan", []typ.Method{
		{Name: "case_receive", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
		{Name: "send", Type: typ.Func().Param("self", typ.Self).Param("v", typ.Boolean).Build()},
	})
	opsChanType := typ.NewInterface("OpsChan", []typ.Method{
		{Name: "case_receive", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})

	chManifest := testutil.ChannelManifest()

	source := `
		function test(stop_signal: BoolChan, ops_channel: OpsChan)
			local result = channel.select {
				stop_signal:case_receive(),
				ops_channel:case_receive(),
			}

			if result.channel == stop_signal then
				return "stopped"
			end

			if result.channel == ops_channel then
				-- stop_signal variable should still be BoolChan, not never
				stop_signal:send(true)
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest),
		testutil.WithTypes(map[string]typ.Type{
			"BoolChan": boolChanType,
			"OpsChan":  opsChanType,
		}))

	t.Log("=== Diagnostic Analysis ===")
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("ERROR at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - channel variable should not be narrowed to never")
	}
}

// TestChannelSelectNarrowing_ChannelVariableNotNarrowed_WhileLoop tests the same pattern
// inside a while loop, matching the exact wippy bus_pattern.lua structure.
func TestChannelSelectNarrowing_ChannelVariableNotNarrowed_WhileLoop(t *testing.T) {
	// Create channel types with send method
	boolChanType := typ.NewInterface("BoolChan", []typ.Method{
		{Name: "case_receive", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
		{Name: "send", Type: typ.Func().Param("self", typ.Self).Param("v", typ.Boolean).Build()},
	})
	opsChanType := typ.NewInterface("OpsChan", []typ.Method{
		{Name: "case_receive", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})

	chManifest := testutil.ChannelManifest()

	source := `
		function test(stop_signal: BoolChan, ops_channel: OpsChan, bus_done: BoolChan)
			while true do
				local result = channel.select {
					stop_signal:case_receive(),
					ops_channel:case_receive(),
				}

				if result.channel == stop_signal then
					bus_done:send(true)
					return
				end

				if result.channel == ops_channel then
					-- After excluding stop_signal from result.channel,
					-- stop_signal variable itself should still be usable
					stop_signal:send(true)
				end
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest),
		testutil.WithTypes(map[string]typ.Type{
			"BoolChan": boolChanType,
			"OpsChan":  opsChanType,
		}))

	t.Log("=== Diagnostic Analysis ===")
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("ERROR at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors - channel variable should not be narrowed to never in loop")
	}
}

// TestChannelSelectIdentity tests that channel.select preserves channel identity.
// When using select with case_receive, the result.channel field should be typed
// as the exact channel type from each case, enabling identity-based narrowing.
//
// Pattern:
//
//	result = select({ch1:case_receive(), ch2:case_receive()})
//	if result.channel == ch1 then result.value : string
//	if result.channel == ch2 then result.value : number
func TestChannelSelectIdentity(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		function test(ch1: Channel<string>, ch2: Channel<number>)
			local result = channel.select {
				ch1:case_receive(),
				ch2:case_receive(),
			}

			if result.channel == ch1 then
				-- result.value should be narrowed to string
				local s: string = result.value
				return "string: " .. s
			end

			if result.channel == ch2 then
				-- result.value should be narrowed to number
				local n: number = result.value
				return "number: " .. tostring(n)
			end

			return "unexpected"
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected identity-based narrowing to work for select results")
	}
}

// TestChannelSelectIdentity_InterfacePayloads ensures identity-based narrowing works
// even when channel element types are interfaces (not records).
// This reproduces the interface+interface case where narrowing currently degrades.
func TestChannelSelectIdentity_InterfacePayloads(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	// Interface payloads (like process.Message / process.Event)
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	eventType := typ.NewInterface("process.Event", []typ.Method{
		{Name: "kind", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})

	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)
	messageChannelType := typ.Instantiate(channelGeneric, messageType)
	eventChannelType := typ.Instantiate(channelGeneric, eventType)

	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "inbox", Type: typ.Func().Returns(messageChannelType).Build()},
		{Name: "events", Type: typ.Func().Returns(eventChannelType).Build()},
	})
	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)
	processManifest.DefineType("Event", eventType)

	source := `
		local inbox_ch = process.inbox()
		local events_ch = process.events()

		local result = channel.select {
			inbox_ch:case_receive(),
			events_ch:case_receive(),
		}

		if result.channel == inbox_ch then
			local msg = result.value
			local t: string = msg:topic()
		end

		if result.channel == events_ch then
			local ev = result.value
			local k: string = ev:kind()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
	)

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected identity-based narrowing to work with interface payloads")
	}
}

// TestAssertNeqSelectNarrowing verifies that assert-style neq functions
// propagate path constraints and narrow select results.
func TestAssertNeqSelectNarrowing(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		local assert = {
			neq = function(a: any, b: any, msg: string?)
				if a == b then
					error(msg or "not equal")
				end
			end
		}

		function test(ch1: Channel<string>, ch2: Channel<number>)
			local result = channel.select {
				ch1:case_receive(),
				ch2:case_receive(),
			}

			assert.neq(result.channel, ch1)

			-- after neq, only ch2 variant should remain
			local n: number = result.value
			return n
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected assert.neq to narrow select result")
	}
}

// TestChannelSelectBranchSafety ensures that narrowing a result.channel branch
// does not corrupt the channel's own type outside the branch.
//
// Pattern: After `if result.channel == ch1 then ... end`, the variable `ch1`
// should retain its original type (not become `never` or be modified).
func TestChannelSelectBranchSafety(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		function test(ch1: Channel<string>, ch2: Channel<number>)
			local result = channel.select {
				ch1:case_receive(),
				ch2:case_receive(),
			}

			if result.channel == ch1 then
				-- Inside this branch, result is narrowed
				local s: string = result.value
			end

			-- After the branch, ch1 must still be Channel<string>
			-- This verifies the channel variable was not corrupted by narrowing
			local v1, ok1 = ch1:receive()
			local str: string = v1

			if result.channel == ch2 then
				local n: number = result.value
			end

			-- ch2 must still be Channel<number>
			local v2, ok2 = ch2:receive()
			local num: number = v2
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected channel variables to retain type after branch narrowing")
	}
}
