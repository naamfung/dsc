package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestModuleGenericInstantiation tests generics from module manifests.
// False positive: using generic type from module fails to instantiate.
func TestModuleGenericInstantiation(t *testing.T) {
	// Create a module with generic types
	m := io.NewManifest("container")

	// Box<T> = {value: T, unwrap: (self) -> T}
	boxElem := typ.NewTypeParam("T", nil)
	boxType := typ.NewInterface("Box", []typ.Method{
		{
			Name: "unwrap",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(boxElem).
				Build(),
		},
	})
	boxGeneric := typ.NewGeneric("Box", []*typ.TypeParam{boxElem}, boxType)
	m.DefineType("Box", boxGeneric)

	// Result<T, E> = {ok: true, value: T} | {ok: false, error: E}
	resultOK := typ.NewTypeParam("T", nil)
	resultErr := typ.NewTypeParam("E", nil)
	resultType := typ.NewUnion(
		typ.NewRecord().Field("ok", typ.True).Field("value", resultOK).Build(),
		typ.NewRecord().Field("ok", typ.False).Field("error", resultErr).Build(),
	)
	resultGeneric := typ.NewGeneric("Result", []*typ.TypeParam{resultOK, resultErr}, resultType)
	m.DefineType("Result", resultGeneric)

	// Module export with wrap function
	moduleType := typ.NewRecord().
		Field("wrap", typ.Func().
			TypeParam("T", nil).
			Param("value", typ.NewTypeParam("T", nil)).
			Returns(typ.Instantiate(boxGeneric, typ.NewTypeParam("T", nil))).
			Build()).
		Build()
	m.SetExport(moduleType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "instantiate generic from module",
			code: `
				local b: Box<number> = {unwrap = function(self): number return 42 end}
				local n: number = b:unwrap()
			`,
			wantError: false,
		},
		{
			name: "use module function returning generic",
			code: `
				local b = container.wrap(42)
				local n = b:unwrap()
			`,
			wantError: false,
		},
		{
			name: "result type pattern matching",
			code: `
				local r: Result<number, string> = {ok = true, value = 42}
				if r.ok then
					local n: number = r.value
				end
			`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("container", m))
			if result.HasError() != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

// TestRegistryTypeLoss tests that registry.get preserves type information.
// False positive: registry.get returns unknown even when type is registered.
func TestRegistryTypeLoss(t *testing.T) {
	// Create a registry module manifest
	m := io.NewManifest("registry")

	// registry.get<T>(key: string): T?
	getFunc := typ.Func().
		TypeParam("T", nil).
		Param("key", typ.String).
		Returns(typ.NewUnion(typ.NewTypeParam("T", nil), typ.Nil)).
		Build()

	// registry.set<T>(key: string, value: T)
	setFunc := typ.Func().
		TypeParam("T", nil).
		Param("key", typ.String).
		Param("value", typ.NewTypeParam("T", nil)).
		Build()

	moduleType := typ.NewRecord().
		Field("get", getFunc).
		Field("set", setFunc).
		Build()
	m.SetExport(moduleType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "registry.get with explicit type annotation",
			code: `
				local val: string? = registry.get("key")
				if val ~= nil then
					local s: string = val
				end
			`,
			wantError: false,
		},
		{
			name: "registry round trip",
			code: `
				registry.set("count", 42)
				local n: number? = registry.get("count")
			`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib(), testutil.WithManifest("registry", m))
			if result.HasError() != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, errors=%v",
					tt.wantError, result.HasError(), testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}
