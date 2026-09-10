package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRegression_TableFieldFunctionErrorReturnCorrelation(t *testing.T) {
	isNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.IsNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)

	assertManifest := io.NewManifest("assert2")
	assertManifest.SetExport(typ.NewRecord().
		Field("is_nil", typ.Func().
			Param("value", typ.Any).
			WithRefinement(isNilEffect).
			Build()).
		Build())

	source := `
local assert = require("assert2")

local mod = {}
function mod.request(ok)
	if ok then
		return { value = "ok" }
	end
	return nil, { code = "bad", message = "failed" }
end

local response, err = mod.request(false)
assert.is_nil(response)
local code = err.code
return code
`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("assert2", assertManifest),
	)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
