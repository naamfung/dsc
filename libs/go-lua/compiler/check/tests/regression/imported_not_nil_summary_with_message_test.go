package regression

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestImportedNotNilSummaryNarrowsWithMessageArg(t *testing.T) {
	assertManifest := io.NewManifest("test")
	assertExport := typ.NewRecord().
		Field("not_nil", typ.Func().
			Param("x", typ.Any).
			OptParam("msg", typ.String).
			Build()).
		Build()
	assertManifest.SetExport(assertExport)

	s := io.NewSummary([]typ.Type{typ.Any, typ.NewOptional(typ.String)}, nil)
	s.Ensures = constraint.FromConstraints(constraint.NotNil{Path: constraint.ParamPath(0)})
	assertManifest.DefineSummary("not_nil", s)

	source := `
		local test = require("test")

		local id: string? = "x"
		test.not_nil(id, "id expected")
		local ok: string = id
		return ok
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("test", assertManifest))
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
