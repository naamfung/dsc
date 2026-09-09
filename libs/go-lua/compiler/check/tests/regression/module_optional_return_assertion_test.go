package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestRegression_ModuleOptionalReturn_NarrowsAfterImportedNotNil(t *testing.T) {
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

local id = repo.create()
test.not_nil(id, "id should be present")
return id:sub(1, 1)
`

	result := testutil.Check(consumer,
		testutil.WithStdlib(),
		testutil.WithModule("repo_mod", repoModule),
		testutil.WithModule("test_mod", assertModule),
	)
	if result.HasError() {
		t.Fatalf("expected no errors after imported not_nil narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRegression_ModuleOptionalReturn_RequiresGuardWithoutAssertion(t *testing.T) {
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

	consumer := `
local repo = require("repo_mod")

local id = repo.create()
return id:sub(1, 1)
`

	result := testutil.Check(consumer,
		testutil.WithStdlib(),
		testutil.WithModule("repo_mod", repoModule),
	)
	if !result.HasError() {
		t.Fatal("expected error when optional return is used without guard")
	}

	msgs := strings.Join(testutil.ErrorMessages(result.Diagnostics), " | ")
	if !strings.Contains(msgs, "optional") {
		t.Fatalf("expected optional-usage diagnostic, got: %s", msgs)
	}
}
