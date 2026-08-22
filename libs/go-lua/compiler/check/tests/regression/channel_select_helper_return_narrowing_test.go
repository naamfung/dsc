package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Regression guard for temporal-style helper functions:
// local helper does channel.select over Event and Time channels, returns nil on timeout,
// and returns event otherwise. Callsite should see event (after nil-check), not time.Time.
func TestChannelSelectHelperReturnNarrowing(t *testing.T) {
	eventType := typ.NewRecord().
		Field("kind", typ.String).
		Field("from", typ.String).
		OptField("result", typ.Any).
		Build()
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
	})

	chManifest := testutil.ChannelManifest()
	channelGen, ok := chManifest.LookupType("Channel")
	if !ok {
		t.Fatal("missing channel.Channel generic")
	}
	channelGeneric, ok := channelGen.(*typ.Generic)
	if !ok {
		t.Fatalf("channel.Channel is not generic: %T", channelGen)
	}
	eventChannelType := typ.Instantiate(channelGeneric, eventType)
	timeChannelType := typ.Instantiate(channelGeneric, timeType)

	processManifest := io.NewManifest("process")
	processManifest.SetExport(typ.NewInterface("process", []typ.Method{
		{Name: "events", Type: typ.Func().Returns(eventChannelType).Build()},
	}))

	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(typ.NewInterface("time", []typ.Method{
		{Name: "after", Type: typ.Func().Param("duration", typ.String).Returns(timeChannelType).Build()},
	}))

	source := `
local function wait_for_exit(timeout)
	local events_ch = process.events()
	local deadline = time.after(timeout)
	local result = channel.select {
		events_ch:case_receive(),
		deadline:case_receive(),
	}
	if result.channel == deadline then
		return nil, "timeout"
	end
	local event = result.value
	return event, nil
end

local e, err = wait_for_exit("5s")
if e ~= nil then
	local k = e.result
end
`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
		testutil.WithManifest("time", timeManifest),
	)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	// Also assert the local function type itself is present and has a usable
	// non-nil primary return slot (regression guard against nil/never collapse).
	sess := result.Session
	if sess == nil || sess.RootResult == nil || sess.RootResult.Graph == nil {
		t.Fatal("missing session root result")
	}
	root := sess.RootResult.Graph
	parentHash := sess.Store.GraphParentHashOf(root.ID())
	parent := sess.Store.Parents()[parentHash]
	funcTypes := sess.Store.GetLocalFuncTypesSnapshot(root, parent)

	var helperFn *typ.Function
	for sym, tpe := range funcTypes {
		if root.NameOf(sym) != "wait_for_exit" {
			continue
		}
		helperFn = unwrap.Function(tpe)
		break
	}
	if helperFn == nil || len(helperFn.Returns) == 0 {
		t.Fatalf("missing wait_for_exit function type in local func snapshot: %v", funcTypes)
	}

	nonNil := narrow.RemoveNil(helperFn.Returns[0])
	if typ.IsNever(nonNil) {
		t.Fatalf("expected non-nil helper return to remain usable, got %v", nonNil)
	}
}

// Regression guard for temporal workflow pattern:
// helper loops over channel.select(Event|Time), accumulates exits[event.from] = event,
// returns the table, then caller indexes entries and accesses sender_exit.result.value.
// If select correlation is lost in loop/fixpoint, map values degrade to time.Time.
func TestChannelSelectHelperExitTablePreservesEventType(t *testing.T) {
	eventRecordType := typ.NewRecord().
		Field("kind", typ.String).
		Field("from", typ.String).
		OptField("result", typ.Any).
		OptField("error", typ.Any).
		OptField("deadline", typ.String).
		Build()
	eventMethodsType := typ.NewInterface("process.EventMethods", []typ.Method{
		{Name: "payload", Type: typ.Func().
			Param("self", typ.Self).
			Returns(typ.NewOptional(typ.Any)).
			Build()},
	})
	eventType := typ.NewAlias("process.Event", typ.NewIntersection(eventRecordType, eventMethodsType))
	eventConsts := typ.NewRecord().
		Field("CANCEL", typ.String).
		Field("EXIT", typ.String).
		Field("LINK_DOWN", typ.String).
		Build()
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
	})

	chManifest := testutil.ChannelManifest()
	channelGen, ok := chManifest.LookupType("Channel")
	if !ok {
		t.Fatal("missing channel.Channel generic")
	}
	channelGeneric, ok := channelGen.(*typ.Generic)
	if !ok {
		t.Fatalf("channel.Channel is not generic: %T", channelGen)
	}
	eventChannelType := typ.Instantiate(channelGeneric, eventType)
	timeChannelType := typ.Instantiate(channelGeneric, timeType)

	processMethods := typ.NewInterface("process", []typ.Method{
		{Name: "events", Type: typ.Func().Returns(eventChannelType).Build()},
	})
	processFields := typ.NewRecord().
		Field("event", eventConsts).
		Build()
	processManifest := io.NewManifest("process")
	processManifest.SetExport(typ.NewIntersection(processMethods, processFields))

	timeManifest := io.NewManifest("time")
	timeManifest.SetExport(typ.NewInterface("time", []typ.Method{
		{Name: "after", Type: typ.Func().Param("duration", typ.String).Returns(timeChannelType).Build()},
	}))

	source := `
local function wait_for_exits(events_ch, pids, timeout)
	local deadline = time.after(timeout or "5s")
	local pending = {}
	local exits = {}
	for i = 1, #pids do
		pending[pids[i]] = true
	end

	while true do
		local has_pending = false
		for _, is_pending in pairs(pending) do
			if is_pending then
				has_pending = true
				break
			end
		end
		if not has_pending then
			return exits, nil
		end

		local result = channel.select {
			events_ch:case_receive(),
			deadline:case_receive(),
		}
		if result.channel == deadline then
			return nil, "timeout waiting for exits"
		end

		local event = result.value
		if event.kind == process.event.EXIT and pending[event.from] then
			exits[event.from] = event
			pending[event.from] = false
		end
	end
end

local events_ch = process.events()
local sender_pid = "sender"
local target_pid = "target"
local exits, exits_err = wait_for_exits(events_ch, { sender_pid, target_pid }, "5s")
if exits == nil then
	return false
end
if exits_err ~= nil then
	return false
end

local sender_exit = exits[sender_pid]
if sender_exit == nil then
	return false
end

local ok = sender_exit.result.value
local okv = sender_exit.result.value.ok
local target_exit = exits[target_pid]
if target_exit == nil then
	return false
end
local topic = target_exit.result.value.received_topic
local payload = target_exit.result.value.received_payload
local source = target_exit.result.value.received_payload.source
`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
		testutil.WithManifest("time", timeManifest),
	)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
