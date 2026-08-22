package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Cross-module regression guard: when a module exports a constructor/open
// function that returns a setmetatable-backed object, exported method surface
// must be preserved for downstream callers.
func TestModuleExport_PreservesSetmetatableMethods(t *testing.T) {
	readerSource := `
		local reader_mt = {}
		reader_mt.__index = reader_mt

		function reader_mt:get_full_context()
			return {}
		end

		function reader_mt:state()
			return { meta = {} }
		end

		local reader = {}

		function reader.open(_id)
			local self = setmetatable({}, reader_mt)
			return self, nil
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
