package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Test spec-narrowing chains: spec() -> method() -> method() -> assert type
func TestSpecNarrowing_ChainedMethodCalls(t *testing.T) {
	// Define types for the chain:
	// Builder has :withName(string) -> BuilderWithName
	// BuilderWithName has :build() -> Product
	// Product has a "name" field of type string

	productType := typ.NewRecord().
		Field("name", typ.String).
		Field("id", typ.Integer).
		Build()

	builderWithNameType := typ.NewInterface("BuilderWithName", []typ.Method{
		{
			Name: "build",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(productType).
				Build(),
		},
	})

	builderType := typ.NewInterface("Builder", []typ.Method{
		{
			Name: "withName",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("name", typ.String).
				Returns(builderWithNameType).
				Build(),
		},
	})

	// create() returns Builder when {typed: true}, any otherwise
	createSpec := contract.NewSpec().
		WithReturnCase(
			constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
				Target: constraint.Path{Root: "$0"},
				Field:  "typed",
				Value:  typ.LiteralBool(true),
			})),
			builderType,
		).
		WithDefaultReturn(typ.Any)

	factoryModule := typ.NewRecord().
		Field("create", typ.Func().
			OptParam("opts", typ.NewRecord().OptField("typed", typ.Boolean).Build()).
			Returns(typ.Any).
			Spec(createSpec).
			Build()).
		Build()

	factoryManifest := io.NewManifest("factory")
	factoryManifest.SetExport(factoryModule)
	factoryManifest.DefineType("Builder", builderType)
	factoryManifest.DefineType("BuilderWithName", builderWithNameType)
	factoryManifest.DefineType("Product", productType)

	source := `
		local builder = factory.create({typed = true})
		local withName = builder:withName("test")
		local product = withName:build()
		local name: string = product.name
		local id: integer = product.id
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("factory", factoryManifest))
	if result.HasError() {
		t.Errorf("expected no errors in chained spec narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Test that spec narrowing without the flag uses default type
func TestSpecNarrowing_ChainedMethodCalls_NoFlag(t *testing.T) {
	productType := typ.NewRecord().
		Field("name", typ.String).
		Build()

	builderWithNameType := typ.NewInterface("BuilderWithName", []typ.Method{
		{
			Name: "build",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(productType).
				Build(),
		},
	})

	builderType := typ.NewInterface("Builder", []typ.Method{
		{
			Name: "withName",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("name", typ.String).
				Returns(builderWithNameType).
				Build(),
		},
	})

	createSpec := contract.NewSpec().
		WithReturnCase(
			constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
				Target: constraint.Path{Root: "$0"},
				Field:  "typed",
				Value:  typ.LiteralBool(true),
			})),
			builderType,
		).
		WithDefaultReturn(typ.Any)

	factoryModule := typ.NewRecord().
		Field("create", typ.Func().
			OptParam("opts", typ.NewRecord().OptField("typed", typ.Boolean).Build()).
			Returns(typ.Any).
			Spec(createSpec).
			Build()).
		Build()

	factoryManifest := io.NewManifest("factory")
	factoryManifest.SetExport(factoryModule)

	// Without typed=true, result is any, method call should work (any has any method)
	source := `
		local builder = factory.create()
		local x = builder:withName("test")
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("factory", factoryManifest))
	if result.HasError() {
		t.Errorf("expected no errors with any type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Test spec-narrowed value flows through a loop
func TestSpecNarrowing_LoopCarriedPropagation(t *testing.T) {
	// Channel with typed receive
	messageType := typ.NewRecord().
		Field("topic", typ.String).
		Field("data", typ.Any).
		Build()

	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	listenSpec := contract.NewSpec().
		WithReturnCase(
			constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
				Target: constraint.Path{Root: "$1"},
				Field:  "typed",
				Value:  typ.LiteralBool(true),
			})),
			typ.Instantiate(channelGeneric, messageType),
		).
		WithDefaultReturn(typ.Instantiate(channelGeneric, typ.Any))

	processModule := typ.NewRecord().
		Field("listen", typ.Func().
			Param("topic", typ.String).
			OptParam("opts", typ.NewRecord().OptField("typed", typ.Boolean).Build()).
			Returns(typ.Instantiate(channelGeneric, typ.Any)).
			Spec(listenSpec).
			Build()).
		Build()

	processManifest := io.NewManifest("process")
	processManifest.SetExport(processModule)
	processManifest.DefineType("Message", messageType)

	// Spec-narrowed channel used in a loop
	source := `
		local ch = process.listen("events", {typed = true})
		local last_topic: string? = nil
		for i = 1, 10 do
			local msg, ok = ch:receive()
			if ok then
				last_topic = msg.topic
			end
		end
		local s: string? = last_topic
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest), testutil.WithManifest("process", processManifest))
	if result.HasError() {
		t.Errorf("expected no errors in loop-carried spec narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Test spec-narrowed value assigned in loop and used after
func TestSpecNarrowing_LoopAssignment(t *testing.T) {
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

	// Accumulate topics from multiple receives in a loop
	source := `
		local ch = process.listen("events", {message = true})
		local topics: string[] = {}
		local count = 0
		while count < 5 do
			local msg, ok = ch:receive()
			if ok then
				topics[#topics + 1] = msg.topic
			end
			count = count + 1
		end
		local first: string? = topics[1]
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest), testutil.WithManifest("process", processManifest))
	if result.HasError() {
		t.Errorf("expected no errors in loop assignment with spec narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Test multi-hop spec narrowing through nested method calls
func TestSpecNarrowing_MultiHopChain(t *testing.T) {
	// Level 3: final result
	resultType := typ.NewRecord().
		Field("value", typ.String).
		Field("count", typ.Integer).
		Build()

	// Level 2: processor with :finish() -> Result
	processorType := typ.NewInterface("Processor", []typ.Method{
		{
			Name: "finish",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(resultType).
				Build(),
		},
	})

	// Level 1: transformer with :process() -> Processor
	transformerType := typ.NewInterface("Transformer", []typ.Method{
		{
			Name: "process",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(processorType).
				Build(),
		},
	})

	// Level 0: pipeline with :transform() -> Transformer
	pipelineType := typ.NewInterface("Pipeline", []typ.Method{
		{
			Name: "transform",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(transformerType).
				Build(),
		},
	})

	// create() with spec narrowing
	createSpec := contract.NewSpec().
		WithReturnCase(
			constraint.FromConjunction(constraint.NewConjunction(constraint.FieldEquals{
				Target: constraint.Path{Root: "$0"},
				Field:  "strict",
				Value:  typ.LiteralBool(true),
			})),
			pipelineType,
		).
		WithDefaultReturn(typ.Any)

	pipeModule := typ.NewRecord().
		Field("create", typ.Func().
			OptParam("opts", typ.NewRecord().OptField("strict", typ.Boolean).Build()).
			Returns(typ.Any).
			Spec(createSpec).
			Build()).
		Build()

	pipeManifest := io.NewManifest("pipe")
	pipeManifest.SetExport(pipeModule)
	pipeManifest.DefineType("Pipeline", pipelineType)
	pipeManifest.DefineType("Transformer", transformerType)
	pipeManifest.DefineType("Processor", processorType)
	pipeManifest.DefineType("Result", resultType)

	// 4-hop chain: create() -> transform() -> process() -> finish() -> assert fields
	source := `
		local pipeline = pipe.create({strict = true})
		local transformer = pipeline:transform()
		local processor = transformer:process()
		local result = processor:finish()
		local value: string = result.value
		local count: integer = result.count
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("pipe", pipeManifest))
	if result.HasError() {
		t.Errorf("expected no errors in multi-hop spec narrowing chain, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
