package regression

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: imported summary EqPath constraints must still narrow when
// arguments are expressions (e.g. #arr), not direct identifier paths.
func TestImportedEqSummaryNarrowsLenBasedIndexing(t *testing.T) {
	assertManifest := io.NewManifest("assert")
	assertExport := typ.NewRecord().
		Field("eq", typ.Func().
			Param("actual", typ.Any).
			Param("expected", typ.Any).
			OptParam("msg", typ.String).
			Build()).
		Build()
	assertManifest.SetExport(assertExport)

	eqSummary := io.NewSummary([]typ.Type{typ.Any, typ.Any, typ.NewOptional(typ.String)}, nil)
	eqSummary.Ensures = constraint.FromConstraints(constraint.EqPath{
		Left:  constraint.ParamPath(0),
		Right: constraint.ParamPath(1),
	})
	assertManifest.DefineSummary("eq", eqSummary)

	source := `
		type Row = { stream: string }
		local assert = require("assert")

		local function parse_stream_lines(raw: string?): {Row}
			local lines = {}
			if raw and raw ~= "" then
				table.insert(lines, { stream = "ok" })
			end
			return lines
		end

		local maybe_raw: string? = "raw"
		local result = parse_stream_lines(maybe_raw)
		assert.eq(#result, 1, "one row")

		local line: string = result[1].stream
		return line
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("assert", assertManifest))
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
