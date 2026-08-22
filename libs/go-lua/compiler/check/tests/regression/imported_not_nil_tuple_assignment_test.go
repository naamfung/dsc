package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestImportedNotNilSummaryNarrowsTupleAssignmentSlot(t *testing.T) {
	repoModule := testutil.CheckAndExport(`
local repo = {}

function repo.create(): (string?, string?)
	return "image-1", nil
end

return repo
`, "repo_mod", testutil.WithStdlib())
	if repoModule.HasError() {
		t.Fatalf("unexpected repo module errors: %v", testutil.ErrorMessages(repoModule.Errors))
	}

	assertModule := testutil.CheckAndExport(`
local test = {}

function test.not_nil(v: any, msg: string?): any
	if v == nil then
		error(msg or "assertion failed")
	end
	return v
end

return test
`, "test_mod", testutil.WithStdlib())
	if assertModule.HasError() {
		t.Fatalf("unexpected assert module errors: %v", testutil.ErrorMessages(assertModule.Errors))
	}

	consumer := `
local repo = require("repo_mod")
local test = require("test_mod")

local id, err = repo.create()
test.not_nil(id, "id expected")
local s: string = id
return s, err
`

	result := testutil.Check(consumer,
		testutil.WithStdlib(),
		testutil.WithModule("repo_mod", repoModule),
		testutil.WithModule("test_mod", assertModule),
	)
	if result.HasError() {
		t.Fatalf("expected no errors after not_nil narrowing on tuple assignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestImportedNilAndNotNilSummariesNarrowSiblingTupleSlots(t *testing.T) {
	repoModule := testutil.CheckAndExport(`
local repo = {}

function repo.create(ok: boolean): (string?, string?)
	if ok then
		return "image-1", nil
	end
	return nil, "failed"
end

return repo
`, "repo_mod", testutil.WithStdlib())
	if repoModule.HasError() {
		t.Fatalf("unexpected repo module errors: %v", testutil.ErrorMessages(repoModule.Errors))
	}

	assertModule := testutil.CheckAndExport(`
local test = {}

function test.not_nil(v: any, msg: string?): any
	if v == nil then
		error(msg or "assertion failed")
	end
	return v
end

function test.is_nil(v: any, msg: string?)
	if v ~= nil then
		error(msg or "assertion failed")
	end
end

return test
`, "test_mod", testutil.WithStdlib())
	if assertModule.HasError() {
		t.Fatalf("unexpected assert module errors: %v", testutil.ErrorMessages(assertModule.Errors))
	}

	consumer := `
local repo = require("repo_mod")
local test = require("test_mod")

local id, err = repo.create(true)
test.is_nil(err, "no error expected")
test.not_nil(id, "id expected")
local s: string = id
return s
`

	result := testutil.Check(consumer,
		testutil.WithStdlib(),
		testutil.WithModule("repo_mod", repoModule),
		testutil.WithModule("test_mod", assertModule),
	)
	if result.HasError() {
		t.Fatalf("expected no errors after sibling nil/not_nil narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestImportedNotNilSummaryWithErrorLevelArgNarrows(t *testing.T) {
	assertModule := testutil.CheckAndExport(`
local test = {}

function test.not_nil(val: any, msg: string?): any
	if val == nil then
		error((msg or "assertion failed") .. ": expected non-nil value", 2)
	end
	return val
end

return test
`, "test_mod", testutil.WithStdlib())
	if assertModule.HasError() {
		t.Fatalf("unexpected assert module errors: %v", testutil.ErrorMessages(assertModule.Errors))
	}

	consumer := `
local test = require("test_mod")
local id: string? = "ok"
test.not_nil(id, "id")
local s: string = id
return s
`

	result := testutil.Check(consumer,
		testutil.WithStdlib(),
		testutil.WithModule("test_mod", assertModule),
	)
	if result.HasError() {
		t.Fatalf("expected no errors after imported not_nil (error level arg), got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
