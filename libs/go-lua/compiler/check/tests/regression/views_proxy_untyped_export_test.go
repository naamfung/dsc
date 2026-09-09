package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
)

// Mirrors framework/views pattern: imported test.not_nil on a field path from
// an untyped/variant-return module must narrow before later field access.
func TestRegression_ViewsProxyNarrowing_FromUntypedVariantModule(t *testing.T) {
	testModuleSource := `
local test = {}

function test.is_nil(val: any, msg: string?)
	if val ~= nil then
		error((msg or "assertion failed") .. ": expected nil", 2)
	end
end

function test.not_nil(val: any, msg: string?): any
	if val == nil then
		error((msg or "assertion failed") .. ": expected non-nil", 2)
	end
	return val
end

return test
`
	testModule := testutil.CheckAndExport(testModuleSource, "test", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("test module should export cleanly, got: %v", testutil.ErrorMessages(testModule.Errors))
	}
	encTest, err := io.EncodeManifest(testModule.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest(test) failed: %v", err)
	}
	decTest, err := io.DecodeManifest(encTest)
	if err != nil {
		t.Fatalf("DecodeManifest(test) failed: %v", err)
	}
	notNilSummary, ok := decTest.LookupSummary("not_nil")
	if !ok || notNilSummary == nil || !notNilSummary.Ensures.HasConstraints() {
		t.Fatalf("expected test.not_nil summary with ensures, got: %#v", notNilSummary)
	}

	pageRegistrySource := `
local M = {}

function M.get(id: string)
	if id == "template" then
		return { kind = "template" }, nil
	end
	return { kind = "component", proxy = { enabled = true, css = { fonts = true } } }, nil
end

return M
`
	registryModule := testutil.CheckAndExport(pageRegistrySource, "page_registry", testutil.WithStdlib())
	if registryModule.HasError() {
		t.Fatalf("page_registry module should export cleanly, got: %v", testutil.ErrorMessages(registryModule.Errors))
	}
	encRegistry, err := io.EncodeManifest(registryModule.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest(page_registry) failed: %v", err)
	}
	decRegistry, err := io.DecodeManifest(encRegistry)
	if err != nil {
		t.Fatalf("DecodeManifest(page_registry) failed: %v", err)
	}

	consumerSource := `
local test = require("test")
local page_registry = require("page_registry")

local page, err = page_registry.get("component")
test.is_nil(err)
test.not_nil(page.proxy)
local enabled: boolean = page.proxy.enabled
local fonts: boolean = page.proxy.css.fonts
return enabled and fonts
`
	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithManifest("test", decTest),
		testutil.WithManifest("page_registry", decRegistry),
	)
	foundProxyNotNil := false
	foundProxyNotNilWithSymbol := false
	if result.Session != nil && result.Session.RootResult != nil && result.Session.RootResult.FlowInputs != nil {
		for _, ec := range result.Session.RootResult.FlowInputs.EdgeConditions {
			for _, c := range ec.Condition.MustConstraints() {
				if nn, ok := c.(constraint.NotNil); ok {
					p := nn.Path
					if p.Root == "page" && len(p.Segments) == 1 && p.Segments[0].Kind == constraint.SegmentField && p.Segments[0].Name == "proxy" {
						foundProxyNotNil = true
						if p.Symbol != 0 {
							foundProxyNotNilWithSymbol = true
						}
					}
				}
			}
		}
	}
	if !foundProxyNotNil {
		t.Fatalf("expected flow inputs to include NotNil(page.proxy) from imported test.not_nil call")
	}
	if !foundProxyNotNilWithSymbol {
		t.Fatalf("expected NotNil(page.proxy) path to carry symbol identity")
	}
	if result.HasError() {
		t.Fatalf("expected no errors after imported not_nil(field-path), got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Same as above but without the preceding is_nil(err) assertion, to isolate
// whether call-sequence interactions are dropping not_nil field-path narrowing.
func TestRegression_ViewsProxyNarrowing_FromUntypedVariantModule_NoPriorAssert(t *testing.T) {
	testModuleSource := `
local test = {}

function test.not_nil(val: any, msg: string?): any
	if val == nil then
		error((msg or "assertion failed") .. ": expected non-nil", 2)
	end
	return val
end

return test
`
	testModule := testutil.CheckAndExport(testModuleSource, "test", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("test module should export cleanly, got: %v", testutil.ErrorMessages(testModule.Errors))
	}
	encTest, err := io.EncodeManifest(testModule.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest(test) failed: %v", err)
	}
	decTest, err := io.DecodeManifest(encTest)
	if err != nil {
		t.Fatalf("DecodeManifest(test) failed: %v", err)
	}

	pageRegistrySource := `
local M = {}

function M.get(id: string)
	if id == "template" then
		return { kind = "template" }, nil
	end
	return { kind = "component", proxy = { enabled = true, css = { fonts = true } } }, nil
end

return M
`
	registryModule := testutil.CheckAndExport(pageRegistrySource, "page_registry", testutil.WithStdlib())
	if registryModule.HasError() {
		t.Fatalf("page_registry module should export cleanly, got: %v", testutil.ErrorMessages(registryModule.Errors))
	}
	encRegistry, err := io.EncodeManifest(registryModule.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest(page_registry) failed: %v", err)
	}
	decRegistry, err := io.DecodeManifest(encRegistry)
	if err != nil {
		t.Fatalf("DecodeManifest(page_registry) failed: %v", err)
	}

	consumerSource := `
local test = require("test")
local page_registry = require("page_registry")

local page = page_registry.get("component")
test.not_nil(page.proxy)
local enabled: boolean = page.proxy.enabled
local fonts: boolean = page.proxy.css.fonts
return enabled and fonts
`
	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithManifest("test", decTest),
		testutil.WithManifest("page_registry", decRegistry),
	)
	if result.HasError() {
		t.Fatalf("expected no errors after imported not_nil(field-path), got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Mirrors views/page_registry construction style where fields are assigned to a
// pre-built table after initialization.
func TestRegression_ViewsProxyNarrowing_FromMutatedTableReturn(t *testing.T) {
	testModuleSource := `
local test = {}

function test.not_nil(val: any, msg: string?): any
	if val == nil then
		error((msg or "assertion failed") .. ": expected non-nil", 2)
	end
	return val
end

return test
`
	testModule := testutil.CheckAndExport(testModuleSource, "test", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("test module should export cleanly, got: %v", testutil.ErrorMessages(testModule.Errors))
	}
	encTest, err := io.EncodeManifest(testModule.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest(test) failed: %v", err)
	}
	decTest, err := io.DecodeManifest(encTest)
	if err != nil {
		t.Fatalf("DecodeManifest(test) failed: %v", err)
	}

	pageRegistrySource := `
local M = {}

function M.get(id: string)
	local page = { kind = id }
	if id == "component" then
		page.proxy = { enabled = true, css = { fonts = true } }
	end
	return page, nil
end

return M
`
	registryModule := testutil.CheckAndExport(pageRegistrySource, "page_registry", testutil.WithStdlib())
	if registryModule.HasError() {
		t.Fatalf("page_registry module should export cleanly, got: %v", testutil.ErrorMessages(registryModule.Errors))
	}
	encRegistry, err := io.EncodeManifest(registryModule.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest(page_registry) failed: %v", err)
	}
	decRegistry, err := io.DecodeManifest(encRegistry)
	if err != nil {
		t.Fatalf("DecodeManifest(page_registry) failed: %v", err)
	}

	consumerSource := `
local test = require("test")
local page_registry = require("page_registry")

local page = page_registry.get("component")
test.not_nil(page.proxy)
local enabled: boolean = page.proxy.enabled
local fonts: boolean = page.proxy.css.fonts
return enabled and fonts
`
	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithManifest("test", decTest),
		testutil.WithManifest("page_registry", decRegistry),
	)
	if result.HasError() {
		t.Fatalf("expected no errors for mutated-table proxy narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRegression_ViewsProxyNarrowing_FromUnknownMetaBranch(t *testing.T) {
	testModuleSource := `
local test = {}

function test.not_nil(val: any, msg: string?): any
	if val == nil then
		error((msg or "assertion failed") .. ": expected non-nil", 2)
	end
	return val
end

return test
`
	testModule := testutil.CheckAndExport(testModuleSource, "test", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("test module should export cleanly, got: %v", testutil.ErrorMessages(testModule.Errors))
	}
	encTest, err := io.EncodeManifest(testModule.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest(test) failed: %v", err)
	}
	decTest, err := io.DecodeManifest(encTest)
	if err != nil {
		t.Fatalf("DecodeManifest(test) failed: %v", err)
	}

	pageRegistrySource := `
local M = {}

local function detect_kind(entry_kind)
	if entry_kind == "template.jet" then
		return "template"
	end
	return "component"
end

function M.get(entry_kind: string)
	local kind = detect_kind(entry_kind)
	local page = {
		id = "id",
		kind = kind,
		content_type = "text/html",
	}

	if kind == "template" then
		page.template_name = "contact"
	else
		page.url = "/index.html"
		page.proxy = { enabled = true, css = { fonts = true } }
	end

	return page, nil
end

return M
`
	registryModule := testutil.CheckAndExport(pageRegistrySource, "page_registry", testutil.WithStdlib())
	if registryModule.HasError() {
		t.Fatalf("page_registry module should export cleanly, got: %v", testutil.ErrorMessages(registryModule.Errors))
	}
	encRegistry, err := io.EncodeManifest(registryModule.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest(page_registry) failed: %v", err)
	}
	decRegistry, err := io.DecodeManifest(encRegistry)
	if err != nil {
		t.Fatalf("DecodeManifest(page_registry) failed: %v", err)
	}

	consumerSource := `
local test = require("test")
local page_registry = require("page_registry")

local page = page_registry.get("view.component")
test.not_nil(page.proxy)
local enabled: boolean = page.proxy.enabled
local fonts: boolean = page.proxy.css.fonts
return enabled and fonts
`
	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithManifest("test", decTest),
		testutil.WithManifest("page_registry", decRegistry),
	)
	if result.HasError() {
		t.Fatalf("expected no errors for unknown-meta branch narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
