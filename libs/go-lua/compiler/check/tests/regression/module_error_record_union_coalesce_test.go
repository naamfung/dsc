package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Regression: when return-slot joins build `(nil | recA) | recB`, compatible
// error record members must be coalesced before export/import so assert-based
// nil checks can correlate sibling returns without spurious field-missing errors.
func TestRegression_ModuleErrorReturnUnionCoalescesAcrossBoundary(t *testing.T) {
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

	moduleSource := `
local M = {}

function M.request(mode: number)
	if mode == 0 then
		return { ok = true }, nil
	end
	if mode == 1 then
		return nil, {
			status_code = 401,
			message = "missing key",
		}
	end
	return nil, {
		status_code = 404,
		message = "Model not found",
		code = "model_not_found",
		type = "invalid_request_error",
	}
end

return M
`

	mod := testutil.CheckAndExport(moduleSource, "mod", testutil.WithStdlib())
	if mod.HasError() {
		t.Fatalf("unexpected module export errors: %v", testutil.ErrorMessages(mod.Errors))
	}

	consumerSource := `
local assert = require("assert2")
local mod = require("mod")

local function run(mode: number)
	local response, err = mod.request(mode)
	assert.is_nil(response)
	return err.code, err.type
end

return run
`

	result := testutil.Check(consumerSource,
		testutil.WithStdlib(),
		testutil.WithManifest("assert2", assertManifest),
		testutil.WithManifest("mod", mod.Manifest),
	)
	if result.HasError() {
		t.Fatalf("expected no errors across module boundary, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
