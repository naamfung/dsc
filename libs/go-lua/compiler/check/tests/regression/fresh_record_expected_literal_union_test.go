package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestFreshRecordExpectedLiteralUnion_ReturnContext(t *testing.T) {
	result := testutil.Check(`
type StatusCode = 200 | 201 | 400
type Response = {
	status: StatusCode,
	body: any?,
	headers: {[string]: string},
}

local function ok(body: any?): Response
	return {status = 200, body = body, headers = {}}
end

local function created(body: any?): Response
	return {status = 201, body = body, headers = {}}
end
`, testutil.WithStdlib())

	if result.HasError() {
		t.Fatalf("expected no errors for fresh return literals under expected record type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFreshRecordExpectedLiteralUnion_AssignmentContext(t *testing.T) {
	result := testutil.Check(`
type StatusCode = 200 | 201 | 400
type Response = {
	status: StatusCode,
	body: any?,
	headers: {[string]: string},
}

local ok: Response = {status = 200, body = nil, headers = {}}
local created: Response = {status = 201, body = nil, headers = {}}

return ok, created
`, testutil.WithStdlib())

	if result.HasError() {
		t.Fatalf("expected no errors for fresh assignment literals under expected record type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
