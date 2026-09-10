package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
)

// Regression guard: inferred assert-style refinements from module source must
// survive export -> manifest encoding -> manifest decoding so downstream module
// consumers still get narrowing at call sites.
func TestRegression_ModuleExportPreservesInferredNotNilRefinement(t *testing.T) {
	moduleSource := `
local M = {}

function M.not_nil(v, msg)
	if v == nil then
		error(msg or "assertion failed")
	end
	return v
end

return M
`

	exported := testutil.CheckAndExport(moduleSource, "assert_mod", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatalf("unexpected module export errors: %v", testutil.ErrorMessages(exported.Errors))
	}

	encoded, err := io.EncodeManifest(exported.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	consumer := `
local assert_mod = require("assert_mod")

local function maybe_string(): string?
	if _G.flag then
		return "ok"
	end
	return nil
end

local x = maybe_string()
assert_mod.not_nil(x, "x is required")
return x:sub(1, 1)
`

	result := testutil.Check(consumer,
		testutil.WithStdlib(),
		testutil.WithManifest("assert_mod", decoded),
	)
	if result.HasError() {
		t.Fatalf("expected no errors with imported inferred refinement, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard: inferred imported not_nil refinements must narrow nested
// field paths (e.g. page.proxy) just like explicit manifest summaries.
func TestRegression_ModuleExportPreservesInferredNotNilFieldPathRefinement(t *testing.T) {
	moduleSource := `
local M = {}

function M.not_nil(v, msg)
	if v == nil then
		error(msg or "assertion failed")
	end
	return v
end

return M
`

	exported := testutil.CheckAndExport(moduleSource, "assert_mod", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatalf("unexpected module export errors: %v", testutil.ErrorMessages(exported.Errors))
	}

	encoded, err := io.EncodeManifest(exported.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	consumer := `
local assert_mod = require("assert_mod")

type Proxy = { enabled: boolean }
type Page = { proxy: Proxy? }

local page: Page = { proxy = { enabled = true } }
assert_mod.not_nil(page.proxy, "proxy required")
return page.proxy.enabled
`

	result := testutil.Check(consumer,
		testutil.WithStdlib(),
		testutil.WithManifest("assert_mod", decoded),
	)
	if result.HasError() {
		t.Fatalf("expected no errors with inferred imported field-path refinement, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard: typed assertion helpers (explicit any/string? annotations)
// must still export inferred not_nil summaries for downstream narrowing.
func TestRegression_ModuleExportPreservesTypedNotNilFieldPathRefinement(t *testing.T) {
	moduleSource := `
local M = {}

function M.not_nil(v: any, msg: string?): any
	if v == nil then
		error(msg or "assertion failed")
	end
	return v
end

return M
`

	exported := testutil.CheckAndExport(moduleSource, "assert_mod_typed", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatalf("unexpected module export errors: %v", testutil.ErrorMessages(exported.Errors))
	}

	encoded, err := io.EncodeManifest(exported.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	consumer := `
local assert_mod_typed = require("assert_mod_typed")

type Proxy = { enabled: boolean }
type Page = { proxy: Proxy? }

local page: Page = { proxy = { enabled = true } }
assert_mod_typed.not_nil(page.proxy, "proxy required")
return page.proxy.enabled
`

	result := testutil.Check(consumer,
		testutil.WithStdlib(),
		testutil.WithManifest("assert_mod_typed", decoded),
	)
	if result.HasError() {
		t.Fatalf("expected no errors with typed inferred field-path refinement, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
