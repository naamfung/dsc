package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Regression: local functions returning `(value?, custom_error_record?)` must
// produce ErrorReturn correlation so assert-based nil checks narrow siblings.
func TestRegression_CustomErrorReturnCorrelationFromBody(t *testing.T) {
	isNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.IsNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)

	assertManifest := io.NewManifest("assert2")
	assertManifest.SetExport(typ.NewRecord().
		Field("is_nil", typ.Func().
			Param("value", typ.Any).
			OptParam("msg", typ.String).
			WithRefinement(isNilEffect).
			Build()).
		Build())

	source := `
local assert = require("assert2")

local function request(ok)
	if ok then
		return { content = "ok" }, nil
	end
	return nil, {
		status_code = 400,
		message = "bad request",
	}
end

local response, err = request(false)
assert.is_nil(response, "response nil on failure")
local status = err.status_code
local message = err.message
return status, message
`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("assert2", assertManifest),
	)
	if result.HasError() {
		t.Fatalf("expected no errors for custom error-return correlation, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
