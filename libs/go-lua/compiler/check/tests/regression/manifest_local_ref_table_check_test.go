package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Guards dependency-context table checks where manifest-local named refs must be
// materialized before nested table compatibility checks.
func TestManifestLocalRef_TableLiteralAssignment(t *testing.T) {
	manifest := io.NewManifest("facade")
	customization := typ.NewRecord().
		Field("custom_css", typ.String).
		Field("css_variables", typ.NewMap(typ.String, typ.String)).
		Field("icons", typ.NewMap(typ.String, typ.String)).
		Build()
	facadeConfig := typ.NewRecord().
		Field("customization", typ.NewRef("", "Customization")).
		Build()
	manifest.DefineType("Customization", customization)
	manifest.DefineType("FacadeConfig", facadeConfig)
	manifest.SetExport(typ.NewRecord().Build())

	source := `
type FacadeConfig = facade.FacadeConfig

local config: FacadeConfig = {
	customization = {
		custom_css = "",
		css_variables = {},
		icons = {},
	},
}
`

	result := testutil.Check(
		source,
		testutil.WithStdlib(),
		testutil.WithManifest("facade", manifest),
	)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
