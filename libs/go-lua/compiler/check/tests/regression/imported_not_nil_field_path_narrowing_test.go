package regression

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: imported not_nil summaries must narrow field paths, not only local identifiers.
func TestImportedNotNilSummaryNarrowsFieldPath(t *testing.T) {
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

		type SystemItem = { text: string }
		type R = { system: {SystemItem}? }
		local result: R = { system = { { text = "ok" } } }

		test.not_nil(result.system, "system required")
		local text: string = result.system[1].text
		return text
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("test", assertManifest))
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard: imported not_nil + imported eq(len(...), n) summaries must
// combine for field-path arrays so indexing is valid after length assertion.
func TestImportedNotNilAndEqLenSummaryNarrowFieldArrayIndexing(t *testing.T) {
	assertManifest := io.NewManifest("test")
	assertExport := typ.NewRecord().
		Field("not_nil", typ.Func().
			Param("x", typ.Any).
			OptParam("msg", typ.String).
			Build()).
		Field("eq", typ.Func().
			Param("actual", typ.Any).
			Param("expected", typ.Any).
			OptParam("msg", typ.String).
			Build()).
		Build()
	assertManifest.SetExport(assertExport)

	notNilSummary := io.NewSummary([]typ.Type{typ.Any, typ.NewOptional(typ.String)}, nil)
	notNilSummary.Ensures = constraint.FromConstraints(constraint.NotNil{Path: constraint.ParamPath(0)})
	assertManifest.DefineSummary("not_nil", notNilSummary)

	eqSummary := io.NewSummary([]typ.Type{typ.Any, typ.Any, typ.NewOptional(typ.String)}, nil)
	eqSummary.Ensures = constraint.FromConstraints(constraint.EqPath{
		Left:  constraint.ParamPath(0),
		Right: constraint.ParamPath(1),
	})
	assertManifest.DefineSummary("eq", eqSummary)

	source := `
		local test = require("test")

		type SystemItem = { text: string }
		type R = { system: {SystemItem}? }
		local result: R = { system = { { text = "ok" } } }

		test.not_nil(result.system, "system required")
		test.eq(#result.system, 1, "one system item")
		local text: string = result.system[1].text
		return text
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("test", assertManifest))
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard: imported not_nil field-path narrowing must work inside
// nested callback bodies (e.g. test.it(..., function() ... end)).
func TestImportedNotNilSummaryNarrowsFieldPathInNestedCallback(t *testing.T) {
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

		local function run(cb: () -> ())
			cb()
		end

		type Proxy = { enabled: boolean }
		type Page = { proxy: Proxy? }
		local page: Page = { proxy = { enabled = true } }

		run(function()
			test.not_nil(page.proxy, "proxy required")
			local enabled: boolean = page.proxy.enabled
			return enabled
		end)
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("test", assertManifest))
	if result.HasError() {
		t.Fatalf("expected no errors in nested callback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
