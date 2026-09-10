package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestFalseOptionalCastToAny_AllowsDirectFieldAccess(t *testing.T) {
	source := `
		local function read_fields(claude_response: false?)
			local resp = claude_response :: any
			local _a = resp.usage
			local _b = resp.stop_reason
			local _c = resp.metadata or {}
			return _a, _b, _c
		end

		read_fields(false)
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalseOptionalCastToAny_LogicalGuardFieldAccess(t *testing.T) {
	source := `
		type Args = { options: false? }

		local function f(contract_args: Args)
			local opts = contract_args.options :: any
			if opts and opts.user then
				return opts.user
			end
			return nil
		end

		f({ options = false })
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
