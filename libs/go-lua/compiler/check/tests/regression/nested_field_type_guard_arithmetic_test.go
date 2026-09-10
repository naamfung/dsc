package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Regression guard: nested field type() checks must make the guarded field
// usable in arithmetic, not collapse it to never.
func TestNestedFieldTypeGuardPreservesArithmetic(t *testing.T) {
	result := testutil.Check(`
type PayloadCarrier = {
	data: fun(self: PayloadCarrier): any,
}

local function bump(carrier: PayloadCarrier?)
	local data = carrier and carrier:data() or nil
	if type(data) ~= "table" or type(data.amount) ~= "number" then
		return nil
	end

	local incremented = data.amount + 1
	local exact: number = data.amount
	return incremented, exact
end

return bump
`, testutil.WithStdlib())

	if result.HasError() {
		t.Fatalf("expected nested field type guard to preserve arithmetic, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestNestedFieldTypeGuardPreservesArithmeticInTemporalLoopShape(t *testing.T) {
	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "from", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})

	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	messageChannelType := typ.Instantiate(channelGeneric, messageType)
	rawChannelType := typ.Instantiate(channelGeneric, typ.Any)

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

	result := testutil.Check(`
local counter = 0
local done = false

coroutine.spawn(function()
	local ch = process.listen("increment", {message = true})
	while not done do
		local msg, ok = ch:receive()
		if not ok then
			break
		end

		local p = msg:payload()
		local data = p and p:data() or nil
		local reply_to = msg:from()

		if type(data) ~= "table" or type(data.amount) ~= "number" then
			process.send(reply_to, "nak", "amount must be a number")
		else
			process.send(reply_to, "ack")
			local amount_sanity = data.amount + 1
			counter = counter + data.amount
			counter = amount_sanity - 1
		end
	end
end)
`,
		testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
	)

	if result.HasError() {
		for _, d := range result.Diagnostics {
			t.Logf("diagnostic at %d:%d: %s", d.Position.Line, d.Position.Column, d.Message)
		}
		t.Fatalf("expected temporal loop shape to preserve nested-field arithmetic, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
