package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Regression: inverse sibling correlation must be applied in the correct
// direction for (value?, err?) pairs.
//
// This mirrors the wippy pattern:
//
//	assert.is_nil(value)
//	assert.not_nil(err)
//
// and ensures the second assertion does not collapse err to never.
func TestRegression_ErrorReturnSiblingCorrelationDirection(t *testing.T) {
	notNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.NotNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)
	isNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.IsNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)

	assertManifest := io.NewManifest("assert2")
	assertManifest.SetExport(typ.NewRecord().
		Field("not_nil", typ.Func().
			Param("value", typ.Any).
			OptParam("msg", typ.String).
			WithRefinement(notNilEffect).
			Build()).
		Field("is_nil", typ.Func().
			Param("value", typ.Any).
			OptParam("msg", typ.String).
			WithRefinement(isNilEffect).
			Build()).
		Build())

	versionType := typ.NewInterface("registry.Version", []typ.Method{
		{Name: "id", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	changesType := typ.NewInterface("registry.Changes", []typ.Method{
		{Name: "apply", Type: typ.Func().
			Param("self", typ.Self).
			Returns(typ.NewOptional(versionType), typ.NewOptional(typ.LuaError)).
			Build()},
	})
	registryManifest := io.NewManifest("registry")
	registryManifest.SetExport(typ.NewRecord().
		Field("changes", typ.Func().Returns(changesType).Build()).
		Build())

	source := `
local assert = require("assert2")
local registry = require("registry")

local changes = registry.changes()

local version, apply_err = changes:apply()
assert.is_nil(version, "version nil on failure")
assert.not_nil(apply_err, "apply error expected")
local kind = apply_err:kind()
local details = apply_err:details()

local ok_version, ok_err = changes:apply()
assert.is_nil(ok_err, "error nil on success")
local id = ok_version:id()

return kind, details, id
`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("assert2", assertManifest),
		testutil.WithManifest("registry", registryManifest),
	)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
