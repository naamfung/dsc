package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Constructors may be declared before metatable methods are added. Exported
// return types must still include methods defined later in the module.
func TestModuleExport_LateMethodEnrichmentRetained(t *testing.T) {
	readerSource := `
		local reader_mt = {
			session_id = nil :: string?,
		}
		reader_mt.__index = reader_mt

		local reader = {}

		function reader.open(id)
			local self = setmetatable({}, reader_mt)
			self.session_id = id
			return self, nil
		end

		function reader_mt:get_full_context()
			return {}
		end

		function reader_mt:state()
			return { meta = {} }
		end

		return reader
	`

	readerModule := testutil.CheckAndExport(readerSource, "reader", testutil.WithStdlib())
	if readerModule.HasError() {
		t.Fatalf("reader module should export cleanly, got: %v", testutil.ErrorMessages(readerModule.Errors))
	}

	consumerSource := `
		local reader = require("reader")
		local r, err = reader.open("s1")
		if err then
			return nil, err
		end
		local full = r:get_full_context()
		local st = r:state()
		return full, st
	`

	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithModule("reader", readerModule),
	)
	if result.HasError() {
		t.Fatalf("expected downstream method calls to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
