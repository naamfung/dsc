package tables

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestChannelSendWidensElementType tests that sending a value to a channel
// widens the channel's element type to include the sent type.
func TestChannelSendWidensElementType(t *testing.T) {
	chManifest := ChannelManifestWithMutation()

	source := `
		local ch = channel.new()  -- Channel<unknown>
		ch:send(42)               -- Should widen to Channel<number>
		local v, ok = ch:receive()
		local n: number = v       -- Should pass if widened correctly
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after channel send widening")
	}
}

// TestChannelSendMultipleTypes tests that sending multiple types unions them.
func TestChannelSendMultipleTypes(t *testing.T) {
	chManifest := ChannelManifestWithMutation()

	source := `
		local ch = channel.new()
		ch:send(42)
		ch:send("hello")
		local v, ok = ch:receive()
		local x: number | string = v  -- Should pass: element type is number|string
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after sending multiple types")
	}
}

// TestChannelSendBranchSafety tests that send in one branch does not affect
// the channel's type after reassignment.
func TestChannelSendBranchSafety(t *testing.T) {
	chManifest := ChannelManifestWithMutation()

	// Test that sends in different branches union correctly
	source := `
		local ch = channel.new()
		if math.random() > 0.5 then
			ch:send("hello")
		else
			ch:send(42)
		end
		local v, ok = ch:receive()
		local x: string | number = v  -- Should pass: element type is string|number
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors for branched sends")
	}
}

// TestChannelSendLiteralWidening ensures literal-only values widen for inference.
func TestChannelSendLiteralWidening(t *testing.T) {
	chManifest := ChannelManifestWithMutation()

	source := `
		local ch = channel.new()
		local x = 0
		if math.random() > 0.5 then
			x = x + 1
		end
		ch:send(x)
		local v, ok = ch:receive()
		local n: integer = v
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after literal widening in channel send")
	}
}

// TestChannelReceiveReturnsWidenedType tests that receive returns the unioned element type.
func TestChannelReceiveReturnsWidenedType(t *testing.T) {
	chManifest := ChannelManifestWithMutation()

	source := `
		local ch = channel.new()
		ch:send({name = "test"})
		local v, ok = ch:receive()
		local name: string = v.name  -- Should pass: value has .name field
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors for receive with widened type")
	}
}

// TestChannelSendDoesNotAffectAnnotatedVar tests that send doesn't widen annotated channels.
func TestChannelSendDoesNotAffectAnnotatedVar(t *testing.T) {
	chManifest := ChannelManifestWithMutation()

	// Annotated variables should not be widened by flow analysis
	source := `
		function test(ch: Channel<number>)
			ch:send(42)
			local v, ok = ch:receive()
			local n: number = v  -- Should pass: annotated type preserved
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors for annotated channel parameter")
	}
}

// TestChannelSendInNestedFunction tests that sends inside a called nested function
// widen the captured channel element type in the parent.
func TestChannelSendInNestedFunction(t *testing.T) {
	chManifest := ChannelManifestWithMutation()

	source := `
		local ch = channel.new()
		local function send()
			ch:send(42)
		end
		send()
		local v, ok = ch:receive()
		local n: number = v
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after nested send widening")
	}
}

// TestChannelSendInSpawnCallback tests that sends inside spawn callbacks
// widen the captured channel element type in the parent.
func TestChannelSendInSpawnCallback(t *testing.T) {
	chManifest := ChannelManifestWithMutation()

	source := `
		local ch = channel.new()
		coroutine.spawn(function()
			ch:send({name = "test"})
		end)
		local v, ok = ch:receive()
		local name: string = v.name
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors after spawn callback send widening")
	}
}

// TestChannelSendWrongType tests that sending wrong type to annotated channel fails.
func TestChannelSendWrongType(t *testing.T) {
	chManifest := ChannelManifestWithMutation()

	// Sending string to Channel<number> should fail
	source := `
		function test(ch: Channel<number>)
			ch:send("hello")  -- Should fail: string not assignable to number
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	if !result.HasError() {
		t.Errorf("expected error when sending wrong type to typed channel")
	}
}

// ChannelManifestWithMutation creates a channel manifest with ContainerElementUnion effect.
func ChannelManifestWithMutation() *io.Manifest {
	m := io.NewManifest("channel")

	// SelectCase<C, T>
	selectCaseType := typ.NewInterface("channel.SelectCase", nil)
	selectCaseChannel := typ.NewTypeParam("C", nil)
	selectCaseValue := typ.NewTypeParam("T", nil)
	selectCaseGeneric := typ.NewGeneric("channel.SelectCase", []*typ.TypeParam{selectCaseChannel, selectCaseValue}, selectCaseType)

	// Channel<T> with send (mutating), receive, case_receive methods
	channelElem := typ.NewTypeParam("T", nil)

	// Create the send spec with ContainerElementUnion effect
	sendSpec := contract.NewSpec().WithEffectRow(effect.Row{
		Labels: []effect.Label{
			effect.Mutate{
				Target: effect.ParamRef{Index: 0}, // self
				Transform: effect.ContainerElementUnion{
					Container: effect.ParamRef{Index: 0}, // self
					Value:     effect.ParamRef{Index: 1}, // value parameter
				},
			},
		},
	})

	// Create the receive spec with Return effect that derives element type from self
	receiveSpec := contract.NewSpec().WithEffects(
		effect.Return{
			ReturnIndex: 0,
			Transform:   effect.ElementOf{Source: effect.ParamRef{Index: 0}}, // elem of self
		},
	)

	channelType := typ.NewInterface("channel.Channel", []typ.Method{
		{
			Name: "send",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("value", channelElem).
				Returns(typ.Boolean).
				Spec(sendSpec).
				Build(),
		},
		{
			Name: "receive",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(channelElem, typ.Boolean).
				Spec(receiveSpec).
				Build(),
		},
		{
			Name: "case_receive",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Instantiate(selectCaseGeneric, typ.Self, channelElem)).
				Build(),
		},
	})
	channelGeneric := typ.NewGeneric("channel.Channel", []*typ.TypeParam{channelElem}, channelType)

	// SelectResult = {channel: any, value: unknown, ok: boolean}
	selectResultType := typ.NewRecord().
		Field("channel", typ.Any).
		Field("value", typ.Unknown).
		Field("ok", typ.Boolean).
		Build()

	m.DefineType("Channel", channelGeneric)
	m.DefineType("SelectCase", selectCaseGeneric)
	m.DefineType("SelectResult", selectResultType)

	channelEmpty := typ.Instantiate(channelGeneric, typ.Unknown)

	moduleType := typ.NewInterface("channel", []typ.Method{
		{Name: "new", Type: typ.Func().OptParam("size", typ.Number).Returns(channelEmpty).Build()},
		{Name: "typed_new", Type: typ.Func().Returns(channelEmpty).Build()},
		{Name: "select", Type: typ.Func().Param("cases", typ.Any).Returns(selectResultType).Build()},
	})
	m.SetExport(moduleType)
	return m
}
