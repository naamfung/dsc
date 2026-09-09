package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Regression guard: imported EqPath summaries must still narrow discriminated
// union fields when one side is a literal argument (for example test.eq(x.kind, "a")).
func TestImportedEqSummaryNarrowsLiteralDiscriminantField(t *testing.T) {
	testManifest := io.NewManifest("test")
	testManifest.SetExport(typ.NewRecord().
		Field("eq", typ.Func().
			Param("actual", typ.Any).
			Param("expected", typ.Any).
			Build()).
		Build())

	eqSummary := io.NewSummary([]typ.Type{typ.Any, typ.Any}, nil)
	eqSummary.Ensures = constraint.FromConstraints(constraint.EqPath{
		Left:  constraint.ParamPath(0),
		Right: constraint.ParamPath(1),
	})
	testManifest.DefineSummary("eq", eqSummary)

	source := `
		local test = require("test")

		type TextPart = { type: "text", text: string }
		type ImagePart = { type: "image", source: string }
		type Part = TextPart | ImagePart

		local system: {Part} = {
			({ type = "text", text = "hello" }) :: Part
		}
		local result = { system = system }

		test.eq(result.system[1].type, "text")
		local s: string = result.system[1].text
		return s
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("test", testManifest),
	)
	if result.HasError() {
		t.Fatalf("expected no errors after eq-literal discriminant narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
