package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// 3) Contract Spec Narrowing (inline literals)

func TestContractSpec_InlineMessageTrue(t *testing.T) {
	messageType := typ.NewRecord().
		Field("topic", typ.String).
		Field("payload", typ.Any).
		Build()

	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

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

	processModule := typ.NewRecord().
		Field("listen", typ.Func().
			Param("topic", typ.String).
			OptParam("opts", typ.NewRecord().OptField("message", typ.Boolean).Build()).
			Returns(typ.Instantiate(channelGeneric, typ.Any)).
			Spec(listenSpec).
			Build()).
		Build()

	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	source := `
		local ch = process.listen("topic", {message = true})
		local msg, ok = ch:receive()
		local topic: string = msg.topic
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest), testutil.WithManifest("process", processManifest))
	if result.HasError() {
		t.Errorf("expected no errors with {message=true}, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestContractSpec_InlineMessageFalse(t *testing.T) {
	messageType := typ.NewRecord().
		Field("topic", typ.String).
		Field("payload", typ.Any).
		Build()

	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

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

	processModule := typ.NewRecord().
		Field("listen", typ.Func().
			Param("topic", typ.String).
			OptParam("opts", typ.NewRecord().OptField("message", typ.Boolean).Build()).
			Returns(typ.Instantiate(channelGeneric, typ.Any)).
			Spec(listenSpec).
			Build()).
		Build()

	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	source := `
		local ch = process.listen("topic", {message = false})
		local val, ok = ch:receive()
		local x = val
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest), testutil.WithManifest("process", processManifest))
	if result.HasError() {
		t.Errorf("expected no errors with {message=false} (default), got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestContractSpec_LocalVarNoNarrowing(t *testing.T) {
	messageType := typ.NewRecord().
		Field("topic", typ.String).
		Field("payload", typ.Any).
		Build()

	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

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

	processModule := typ.NewRecord().
		Field("listen", typ.Func().
			Param("topic", typ.String).
			OptParam("opts", typ.NewRecord().OptField("message", typ.Boolean).Build()).
			Returns(typ.Instantiate(channelGeneric, typ.Any)).
			Spec(listenSpec).
			Build()).
		Build()

	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)

	// When opts is in a local var, spec narrowing does not apply (current behavior lock)
	source := `
		local opts = {message = true}
		local ch = process.listen("topic", opts)
		local val, ok = ch:receive()
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest), testutil.WithManifest("process", processManifest))
	// This should NOT error even without narrowing - val is any
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
