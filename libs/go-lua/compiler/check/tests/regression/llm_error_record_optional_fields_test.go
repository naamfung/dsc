package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRegression_LLMStyleErrorRecordOptionalFields(t *testing.T) {
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
local json = require("json")
local assert = require("assert2")

local function parse_error_response(http_response)
	local error_info = {
		status_code = http_response and http_response.status_code or 0,
		message = "api error",
	}

	if http_response and http_response.body then
		local parsed = json.decode(http_response.body)
		if parsed and parsed.error then
			error_info.message = parsed.error.message or error_info.message
			error_info.code = parsed.error.code
			error_info.type = parsed.error.type
		end
	end

	return error_info
end

local client = {}
function client.request()
	local response = {
		status_code = 404,
		body = "{\"error\": {\"message\": \"m\", \"code\": \"c\", \"type\": \"t\"}}",
	}
	if response.status_code < 200 or response.status_code >= 300 then
		local parsed_error = parse_error_response(response)
		return nil, parsed_error
	end
	return { ok = true }
end

local response, err = client.request()
assert.is_nil(response)
local code = err.code
local kind = err.type
return code, kind
`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("assert2", assertManifest),
	)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
