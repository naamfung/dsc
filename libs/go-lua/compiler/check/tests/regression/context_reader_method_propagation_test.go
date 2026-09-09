package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Session-style context object should preserve receiver methods when a
// setmetatable-backed value is stored in a table field and consumed later.
func TestContextReaderMethodPropagation(t *testing.T) {
	source := `
		local reader_mt = {}
		reader_mt.__index = reader_mt

		function reader_mt:get_full_context()
			return {}
		end

		function reader_mt:state()
			return { meta = {} }
		end

		local function open_reader()
			local self = setmetatable({}, reader_mt)
			return self, nil
		end

		local handlers = {}

		function handlers.step(ctx)
			local full = ctx.reader:get_full_context()
			local st = ctx.reader:state()
			return full, st
		end

		local function run()
			local reader, _ = open_reader()
			local context = { reader = reader }
			return handlers.step(context)
		end

		run()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
