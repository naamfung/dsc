package modules

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestManifestAliasStructuralEquivalence tests that type aliases exported from
// manifests are structurally equivalent to their underlying types.
func TestManifestAliasStructuralEquivalence(t *testing.T) {
	// Build manifest with Name alias (string)
	nameAlias := typ.NewAlias("mod.Name", typ.String)

	// Build manifest with Handler alias (function)
	handlerAlias := typ.NewAlias("mod.Handler", typ.Func().
		Param("s", typ.String).
		Returns(typ.String).
		Build())

	// Build manifest with Point alias (record)
	pointAlias := typ.NewAlias("mod.Point", typ.NewRecord().
		Field("x", typ.Number).
		Field("y", typ.Number).
		Build())

	// Module with functions that use the aliases
	modType := typ.NewRecord().
		Field("greet", typ.Func().
			Param("name", nameAlias).
			Returns(typ.String).
			Build()).
		Field("apply", typ.Func().
			Param("handler", handlerAlias).
			Param("input", typ.String).
			Returns(typ.String).
			Build()).
		Field("distance", typ.Func().
			Param("p1", pointAlias).
			Param("p2", pointAlias).
			Returns(typ.Number).
			Build()).
		Field("make_name", typ.Func().
			Param("s", typ.String).
			Returns(nameAlias).
			Build()).
		Build()

	manifest := io.NewManifest("mod")
	manifest.DefineType("Name", nameAlias)
	manifest.DefineType("Handler", handlerAlias)
	manifest.DefineType("Point", pointAlias)
	manifest.SetExport(modType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "string_literal_to_alias_param",
			code: `
				local msg: string = mod.greet("world")
			`,
			wantError: false,
		},
		{
			name: "function_literal_to_alias_param",
			code: `
				local result: string = mod.apply(function(s: string): string return s:upper() end, "test")
			`,
			wantError: false,
		},
		{
			name: "table_literal_to_alias_param",
			code: `
				local d: number = mod.distance({x = 0, y = 0}, {x = 3, y = 4})
			`,
			wantError: false,
		},
		{
			name: "alias_return_assigned_to_underlying",
			code: `
				local name: string = mod.make_name("hello")
			`,
			wantError: false,
		},
		{
			name: "alias_return_used_as_alias_param",
			code: `
				local name = mod.make_name("hello")
				local msg: string = mod.greet(name)
			`,
			wantError: false,
		},
		{
			name: "wrong_type_rejected",
			code: `
				local msg: string = mod.greet(42)
			`,
			wantError: true,
		},
		{
			name: "wrong_function_signature_rejected",
			code: `
				local result: string = mod.apply(function(n: number): number return n end, "test")
			`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("mod", manifest))
			if result.HasError() != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// TestManifestFunctionAliasCallable tests that function type aliases from manifests
// are recognized as callable.
func TestManifestFunctionAliasCallable(t *testing.T) {
	// Callback = fun(s: string): string
	callbackAlias := typ.NewAlias("mod.Callback", typ.Func().
		Param("s", typ.String).
		Returns(typ.String).
		Build())

	// Module with function that returns a Callback
	modType := typ.NewRecord().
		Field("get_callback", typ.Func().
			Returns(callbackAlias).
			Build()).
		Build()

	manifest := io.NewManifest("mod")
	manifest.DefineType("Callback", callbackAlias)
	manifest.SetExport(modType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "call_function_alias_return",
			code: `
				local cb = mod.get_callback()
				local result: string = cb("hello")
			`,
			wantError: false,
		},
		{
			name: "function_alias_wrong_arg_type",
			code: `
				local cb = mod.get_callback()
				local result: string = cb(42)
			`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithManifest("mod", manifest))
			if result.HasError() != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// TestManifestAliasInChannel tests that type aliases work correctly in channel types.
func TestManifestAliasInChannel(t *testing.T) {
	// Event alias like wippy's process.Event
	eventAlias := typ.NewAlias("mod.Event", typ.NewRecord().
		Field("kind", typ.String).
		Field("from", typ.String).
		OptField("result", typ.Any).
		Build())

	// Get channel generic from stdlib
	chManifest := testutil.ChannelManifest()
	channelGen, _ := chManifest.LookupType("Channel")
	channelGeneric := channelGen.(*typ.Generic)

	// Channel<Event>
	eventChannelType := typ.Instantiate(channelGeneric, eventAlias)

	// Module with events() returning Channel<Event>
	modType := typ.NewRecord().
		Field("events", typ.Func().
			Returns(eventChannelType).
			Build()).
		Build()

	manifest := io.NewManifest("mod")
	manifest.DefineType("Event", eventAlias)
	manifest.SetExport(modType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "receive_from_aliased_channel",
			code: `
				local ch = mod.events()
				local event, ok = ch:receive()
				local kind: string = event.kind
				local from: string = event.from
			`,
			wantError: false,
		},
		{
			name: "access_optional_field_after_receive",
			code: `
				local ch = mod.events()
				local event, ok = ch:receive()
				if event.result then
					local r = event.result
				end
			`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest), testutil.WithManifest("mod", manifest))
			if result.HasError() != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}
