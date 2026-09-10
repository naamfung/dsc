package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Reproduces a false positive from docker log-streamer loops:
// result.channel ~= some_channel should not eliminate all variants when multiple
// select cases share the same channel type instance.
func TestRegression_ChannelSelectNotEquals_DoesNotOverExcludeSameTypeCases(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})

	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)
	messageChannelType := typ.Instantiate(channelGeneric, messageType)

	processModule := typ.NewInterface("process", []typ.Method{
		{Name: "inbox", Type: typ.Func().Returns(messageChannelType).Build()},
		{Name: "events", Type: typ.Func().Returns(messageChannelType).Build()},
		{Name: "leave", Type: typ.Func().Returns(messageChannelType).Build()},
		{Name: "drain", Type: typ.Func().Returns(messageChannelType).Build()},
	})
	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	source := `
		local inbox = process.inbox()
		local events = process.events()
		local leave_ch = process.leave()
		local drain_ch = process.drain()

		local result = channel.select({
			inbox:case_receive(),
			events:case_receive(),
			leave_ch:case_receive(),
			drain_ch:case_receive(),
		})

		if result.channel == events or result.channel == leave_ch then
			return
		end

		if result.channel == inbox then
			local msg = result.value
			local _: string = msg:topic()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(),
		testutil.WithManifest("channel", chManifest),
		testutil.WithManifest("process", processManifest),
	)
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors for channel select narrowing with same-type channels")
	}
}
