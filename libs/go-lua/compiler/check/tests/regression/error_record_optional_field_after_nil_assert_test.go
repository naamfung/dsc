package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Regression: when function return paths build related error records with
// additional fields on some paths, merged return slots should expose those
// fields as optional (instead of producing "field missing on union member").
func TestRegression_ErrorRecordFieldsBecomeOptionalAcrossReturnPaths(t *testing.T) {
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

local function request(flag)
	if flag then
		return nil, {
			status_code = 400,
			message = "bad request",
			code = "invalid_request",
			type = "client_error",
		}
	end
	return nil, {
		status_code = 500,
		message = "internal error",
	}
end

local response, err = request(_G.dynamic_flag)
assert.is_nil(response)
local code = err.code
local typx = err.type
return code, typx
`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("assert2", assertManifest),
	)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
