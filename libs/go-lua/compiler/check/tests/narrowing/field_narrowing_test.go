package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestUnionFieldNarrowing_TruthyErrorCheck tests truthy error field narrowing.
func TestUnionFieldNarrowing_TruthyErrorCheck(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "truthy error field narrows optional to non-nil",
			Code: `
				type Result = {value: number?, error: string?}

				function get_data(): Result
					return {error = "oops", value = nil}
				end

				function process()
					local data = get_data()
					if data.error then
						local e: string = data.error
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "truthy error with else accesses value",
			Code: `
				type Result = {value: number?, error: string?}

				function get_data(): Result
					return {value = 42, error = nil}
				end

				function process()
					local data = get_data()
					if data.error then
						local e: string = data.error
					else
						if data.value then
							local v: number = data.value
						end
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "early return on error narrows optional",
			Code: `
				type Result = {value: number?, error: string?}

				function get_data(): Result
					return {value = 42, error = nil}
				end

				function process(): number
					local data = get_data()
					if data.error then
						return 0
					end
					if data.value then
						return data.value
					end
					return -1
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "optional error field truthy check",
			Code: `
				type Event = {kind: string, error: string?}

				function handle(event: Event)
					if event.error then
						local e: string = event.error
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "union with common optional error field",
			Code: `
				type EventA = {kind: "a", data: number, error: string?}
				type EventB = {kind: "b", msg: string, error: string?}
				type Event = EventA | EventB

				function handle(event: Event)
					if event.error then
						local e: string = event.error
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestUnionFieldNarrowing_EnumInequality tests ~= enum literal narrowing.
func TestUnionFieldNarrowing_EnumInequality(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "kind ~= enum literal excludes variant from union",
			Code: `
				type ExitEvent = {kind: "exit", code: number}
				type MessageEvent = {kind: "message", data: string}
				type Event = ExitEvent | MessageEvent

				function handle(event: Event)
					if event.kind ~= "exit" then
						local k: "message" = event.kind
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "kind ~= enum with early return narrows continuation",
			Code: `
				type ExitEvent = {kind: "exit", code: number}
				type MessageEvent = {kind: "message", data: string}
				type Event = ExitEvent | MessageEvent

				function handle(event: Event): number
					if event.kind ~= "exit" then
						return 0
					end
					return event.code
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "kind == enum literal narrows to matching variant",
			Code: `
				type ExitEvent = {kind: "exit", code: number}
				type MessageEvent = {kind: "message", data: string}
				type Event = ExitEvent | MessageEvent

				function handle(event: Event)
					if event.kind == "exit" then
						local c: number = event.code
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "three variant enum narrowing via nested ~= and ==",
			Code: `
				type A = {kind: "a", a_val: number}
				type B = {kind: "b", b_val: string}
				type C = {kind: "c", c_val: boolean}
				type ABC = A | B | C

				function handle(x: ABC)
					if x.kind ~= "a" then
						if x.kind == "b" then
							local v: string = x.b_val
						else
							local v: boolean = x.c_val
						end
					else
						local v: number = x.a_val
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "enum constant comparison with early return narrows",
			Code: `
				type ExitEvent = {kind: "exit", code: number}
				type CancelEvent = {kind: "cancel"}
				type Event = ExitEvent | CancelEvent

				local EXIT = "exit"

				function handle(event: Event): number
					if event.kind ~= EXIT then
						return 0
					end
					return event.code
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestChannelSelectNarrowingE2E tests channel selection narrowing patterns.
func TestChannelSelectNarrowingE2E(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "select result.error ~= nil narrows optional",
			Code: `
				type Result = {error: string?, value: number?}

				function get_data(): Result
					return {error = nil, value = 42}
				end

				function process()
					local data = get_data()
					if data.error ~= nil then
						local e: string = data.error
						return
					end
					if data.value ~= nil then
						local v: number = data.value
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested result field access after truthy check",
			Code: `
				type Event = {kind: string, result: {value: number}?, error: string?}

				function get_event(): Event
					return {kind = "data", result = {value = 1}, error = nil}
				end

				function process()
					local event = get_event()
					if event.error ~= nil then
						return nil, event.error
					end
					if event.result then
						local r = event.result
						local v: number = r.value
						return v
					end
					return nil
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "channel field equality narrows to correct variant",
			Code: `
				type EventCh = {__tag: "event"}
				type MsgCh = {__tag: "msg"}
				type Event = {kind: string}
				type Msg = {topic: string}

				type SelectResult =
					{channel: EventCh, value: Event, ok: boolean} |
					{channel: MsgCh, value: Msg, ok: boolean}

				function do_select(e: EventCh, m: MsgCh): SelectResult
					return {channel = e, value = {kind = "exit"}, ok = true}
				end

				function process(events_ch: EventCh, msg_ch: MsgCh)
					local result = do_select(events_ch, msg_ch)
					if result.channel == events_ch then
						local k: string = result.value.kind
					else
						local t: string = result.value.topic
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "select with three channels sequential narrowing",
			Code: `
				type EventCh = {__tag: "event"}
				type MsgCh = {__tag: "msg"}
				type TimerCh = {__tag: "timer"}
				type Event = {kind: string}
				type Msg = {topic: string}
				type Timer = {elapsed: number}

				type SelectResult =
					{channel: EventCh, value: Event, ok: boolean} |
					{channel: MsgCh, value: Msg, ok: boolean} |
					{channel: TimerCh, value: Timer, ok: boolean}

				function do_select(e: EventCh, m: MsgCh, t: TimerCh): SelectResult
					return {channel = e, value = {kind = "exit"}, ok = true}
				end

				function process(events_ch: EventCh, msg_ch: MsgCh, timer_ch: TimerCh)
					local result = do_select(events_ch, msg_ch, timer_ch)
					if result.channel == timer_ch then
						return nil, "timeout"
					end
					if result.channel == events_ch then
						local k: string = result.value.kind
						return "event", k
					end
					local t: string = result.value.topic
					return "msg", t
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "result.value.error pattern from wippy",
			Code: `
				type EventCh = {__tag: "event"}
				type TimeoutCh = {__tag: "timeout"}
				type Event = {kind: string, result: any?, error: string?}
				type Time = {sec: number}

				type SelectResult =
					{channel: EventCh, value: Event, ok: boolean} |
					{channel: TimeoutCh, value: Time, ok: boolean}

				function do_select(e: EventCh, t: TimeoutCh): SelectResult
					return {channel = e, value = {kind = "call", result = 42, error = nil}, ok = true}
				end

				function process(events_ch: EventCh, timeout_ch: TimeoutCh)
					local result = do_select(events_ch, timeout_ch)
					if result.channel == timeout_ch then
						return false, "timeout"
					end
					local event = result.value
					if event.error then
						return false, event.error
					end
					return true, event.result
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestLoopNarrowing tests narrowing inside loop constructs.
func TestLoopNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "while with nil check",
			Code: `
				function f(x: string?)
					while x ~= nil do
						local s: string = x
						break
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "repeat until nil check",
			Code: `
				function f(): string
					local x: string? = nil
					repeat
						x = "hello"
					until x ~= nil
					return x
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestIndexEqualityNarrowing tests narrowing via map index equality.
func TestIndexEqualityNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "map index equality narrows base union",
			Code: `
				type A = {[string]: "a"}
				type B = {[string]: "b"}

				function f(t: A | B, k: string)
					if t[k] == "a" then
						local x: A = t
					else
						local y: B = t
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestSelectChannelNarrowing tests channel selection narrowing patterns.
func TestSelectChannelNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "channel equality narrows result value",
			Code: `
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
						local e: string = result.value.error
					else
						local d: number = result.value.data
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "channel inequality narrows result value",
			Code: `
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
					if result.channel ~= ch1 then
						local d: number = result.value.data
					else
						local e: string = result.value.error
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "channel field narrowing ignores local name shadow",
			Code: `
				type ChanInt = {__tag: "int"}
				type ChanStr = {__tag: "str"}
				type SelResult =
					{channel: ChanInt, value: {error: string}, ok: boolean} |
					{channel: ChanStr, value: {data: number}, ok: boolean}

				function get_result(a: ChanInt, b: ChanStr): SelResult
					return {channel = a, value = {error = "oops"}, ok = true}
				end

				function f(ch1: ChanInt, ch2: ChanStr)
					local channel = ch2
					local result = get_result(ch1, ch2)
					if result.channel == ch1 then
						local e: string = result.value.error
					else
						local d: number = result.value.data
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestInequalityUnionNarrowing tests inequality-based union narrowing.
func TestInequalityUnionNarrowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "~= excludes union member by discriminator",
			Code: `
				type ExitEvent = {kind: "exit", code: number}
				type DataEvent = {kind: "data", payload: string}
				type Event = ExitEvent | DataEvent

				function handle(event: Event)
					if event.kind ~= "exit" then
						local p: string = event.payload
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "~= narrows else branch to excluded member",
			Code: `
				type ExitEvent = {kind: "exit", code: number}
				type DataEvent = {kind: "data", payload: string}
				type Event = ExitEvent | DataEvent

				function handle(event: Event)
					if event.kind ~= "exit" then
						local p: string = event.payload
					else
						local c: number = event.code
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "~= with three member union",
			Code: `
				type A = {kind: "a", a: number}
				type B = {kind: "b", b: string}
				type C = {kind: "c", c: boolean}
				type ABC = A | B | C

				function handle(x: ABC)
					if x.kind ~= "a" then
						if x.kind == "b" then
							local s: string = x.b
						else
							local b: boolean = x.c
						end
					else
						local n: number = x.a
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "~= does not narrow when value not in union",
			Code: `
				type ExitEvent = {kind: "exit", code: number}
				type DataEvent = {kind: "data", payload: string}
				type Event = ExitEvent | DataEvent

				function handle(event: Event)
					if event.kind ~= "unknown" then
						local p: string = event.payload
					end
				end
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "~= early return pattern",
			Code: `
				type ExitEvent = {kind: "exit", code: number}
				type DataEvent = {kind: "data", payload: string}
				type Event = ExitEvent | DataEvent

				function handle(event: Event): string
					if event.kind ~= "data" then
						return tostring(event.code)
					end
					return event.payload
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "~= with boolean discriminator",
			Code: `
				type Success = {ok: true, value: number}
				type Failure = {ok: false, error: string}
				type Result = Success | Failure

				function handle(r: Result)
					if r.ok ~= true then
						local e: string = r.error
					else
						local v: number = r.value
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "~= with number discriminator",
			Code: `
				type OK = {status: 200, data: string}
				type NotFound = {status: 404, message: string}
				type Response = OK | NotFound

				function handle(r: Response)
					if r.status ~= 200 then
						local m: string = r.message
					else
						local d: string = r.data
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestNonNilAssertOnNarrowedFieldAccess tests the ! operator on field access
// after discriminated union narrowing via channel select pattern.
func TestNonNilAssertOnNarrowedFieldAccess(t *testing.T) {
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	// Message type with optional data field
	messageType := typ.NewRecord().
		Field("type", typ.String).
		OptField("data", typ.String).
		Build()

	// Time type
	timeType := typ.NewRecord().
		Field("sec", typ.Number).
		Field("nsec", typ.Number).
		Build()

	messageChannelType := typ.Instantiate(channelGeneric, messageType)
	timeChannelType := typ.Instantiate(channelGeneric, timeType)

	// Module that returns Channel<Message>
	wsManifest := io.NewManifest("websocket")
	wsModule := typ.NewInterface("websocket", []typ.Method{
		{Name: "connect", Type: typ.Func().
			Param("url", typ.String).
			Returns(messageChannelType).
			Build()},
	})
	wsManifest.SetExport(wsModule)
	wsManifest.DefineType("Message", messageType)

	// Time module that returns Channel<Time>
	timeManifest := io.NewManifest("time")
	timeModule := typ.NewInterface("time", []typ.Method{
		{Name: "after", Type: typ.Func().
			Param("d", typ.String).
			Returns(timeChannelType).
			Build()},
	})
	timeManifest.SetExport(timeModule)
	timeManifest.DefineType("Time", timeType)

	source := `
		local ch = websocket.connect("ws://localhost:8080")
		local timeout = time.after("5s")

		local result = channel.select {
			ch:case_receive(),
			timeout:case_receive(),
		}

		if result.channel == timeout then
			return nil, "timeout"
		end

		-- After narrowing, result.value should be Message, not Message|Time
		local msg = result.value

		-- First check: msg.data should be string? (optional field)
		local msgDataOpt: string? = msg.data
		-- Second check: msg.data! should be string (nil removed)
		local msgData: string = msg.data!
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("websocket", wsManifest),
		testutil.WithManifest("time", timeManifest),
	)

	if result.Session != nil && result.Session.RootResult != nil {
		graph := result.Session.RootResult.Graph
		synth := result.Session.RootResult.NarrowSynth
		scopes := result.Session.RootResult.Scopes
		if graph != nil && synth != nil {
			graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
				if info == nil {
					return
				}
				for i, src := range info.Sources {
					if attr, ok := src.(*ast.AttrGetExpr); ok {
						if key, ok := attr.Key.(*ast.StringExpr); ok && key.Value == "data" {
							t.Logf("debug: attr-only object type=%v", synth.TypeOf(attr.Object, p))
							t.Logf("debug: attr-only get type=%v (point=%d)", synth.TypeOf(attr, p), p)
						}
					}

					nn, ok := src.(*ast.NonNilAssertExpr)
					if !ok {
						continue
					}
					attr, ok := nn.Expr.(*ast.AttrGetExpr)
					if !ok {
						continue
					}
					key, ok := attr.Key.(*ast.StringExpr)
					if !ok || key.Value != "data" {
						continue
					}
					t.Logf("debug: attr object type=%v", synth.TypeOf(attr.Object, p))
					t.Logf("debug: attr get type=%v", synth.TypeOf(nn.Expr, p))
					t.Logf("debug: non-nil assert type=%v (point=%d)", synth.TypeOf(src, p), p)
					if info.IsLocal && i < len(info.TypeAnnotations) && info.TypeAnnotations[i] != nil {
						sc := scopes[p]
						expected := synth.ResolveType(info.TypeAnnotations[i], sc)
						t.Logf("debug: expected=%v, withExpected=%v", expected, synth.SynthWithExpected(src, p, expected))
					}
				}
			})
		}
	}

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after narrowing and non-nil assert, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
