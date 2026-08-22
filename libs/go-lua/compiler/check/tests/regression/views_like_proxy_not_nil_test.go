package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
)

// Regression guard for views/page_registry_test shape:
// imported test.is_nil + test.not_nil must narrow `page.proxy` in nested test callbacks.
func TestRegression_ViewsLikeProxyNotNilNarrowingInCallback(t *testing.T) {
	testModuleSource := `
local test = {}

function test.is_nil(val: any, msg: string?)
	if val ~= nil then
		error(msg or "expected nil")
	end
end

function test.not_nil(val: any, msg: string?): any
	if val == nil then
		error(msg or "expected non-nil")
	end
	return val
end

function test.it(_name: string, fn: fun())
	fn()
end

return test
`

	testModule := testutil.CheckAndExport(testModuleSource, "test", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("test module should export cleanly, got: %v", testutil.ErrorMessages(testModule.Errors))
	}
	encodedTest, err := io.EncodeManifest(testModule.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest(test) failed: %v", err)
	}
	decodedTest, err := io.DecodeManifest(encodedTest)
	if err != nil {
		t.Fatalf("DecodeManifest(test) failed: %v", err)
	}

	pageRegistrySource := `
type Proxy = { enabled: boolean }
type Page = { proxy: Proxy? }

local M = {}

function M.get(id: string): (Page?, string?)
	if id == "missing" then
		return nil, "not found"
	end
	return { proxy = { enabled = true } }, nil
end

return M
`

	pageRegistry := testutil.CheckAndExport(pageRegistrySource, "page_registry", testutil.WithStdlib())
	if pageRegistry.HasError() {
		t.Fatalf("page_registry module should export cleanly, got: %v", testutil.ErrorMessages(pageRegistry.Errors))
	}
	encodedRegistry, err := io.EncodeManifest(pageRegistry.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest(page_registry) failed: %v", err)
	}
	decodedRegistry, err := io.DecodeManifest(encodedRegistry)
	if err != nil {
		t.Fatalf("DecodeManifest(page_registry) failed: %v", err)
	}

	consumerSource := `
local test = require("test")
local page_registry = require("page_registry")

test.it("proxy defaults", function()
	local page, err = page_registry.get("ok")
	test.is_nil(err)
	test.not_nil(page.proxy)
	local enabled: boolean = page.proxy.enabled
	local _ = enabled
end)
`

	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithManifest("test", decodedTest),
		testutil.WithManifest("page_registry", decodedRegistry),
	)
	if result.HasError() {
		t.Fatalf("expected no errors for views-like proxy narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
