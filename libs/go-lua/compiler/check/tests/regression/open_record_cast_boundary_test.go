package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestOpenRecordCast_AllowsReturnBoundary(t *testing.T) {
	source := `
		local function load_spec(): {}?
			local spec: any = {}
			return spec :: {}?
		end

		local _ = load_spec()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestOpenRecordCast_AllowsCallBoundary(t *testing.T) {
	source := `
		local function consume(ctx: {}?)
			return ctx
		end

		local context: any = {}
		local _ = consume(context :: {}?)
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
