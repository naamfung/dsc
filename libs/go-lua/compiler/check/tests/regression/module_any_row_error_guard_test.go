package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Regression: imported module methods with `(value, err)` shape must preserve
// sibling narrowing at call sites even when value flows from dynamic rows.
func TestRegression_ModuleAnyRowErrorGuardKeepsValueNonNil(t *testing.T) {
	extManifest := io.NewManifest("ext")
	extManifest.SetExport(typ.NewRecord().
		Field("query", typ.Func().
			Param("id", typ.String).
			Returns(typ.NewArray(typ.Any), typ.NewOptional(typ.String)).
			Build()).
		Build())

	repoSource := `
local ext = require("ext")
local M = {}

function M.get(id: string)
	local rows, query_err = ext.query(id)
	if query_err then
		return nil, query_err
	end
	if #rows == 0 then
		return nil, "Task not found"
	end
	return rows[1]
end

return M
`

	repoModule := testutil.CheckAndExport(
		repoSource,
		"repo",
		testutil.WithStdlib(),
		testutil.WithManifest("ext", extManifest),
	)
	if repoModule.HasError() {
		t.Fatalf("repo module should export cleanly, got: %v", testutil.ErrorMessages(repoModule.Errors))
	}

	consumerSource := `
local repo = require("repo")

local task, err = repo.get("t1")
if err then
	return nil, err
end

return task.user_id
`

	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithModule("repo", repoModule),
	)
	if result.HasError() {
		t.Fatalf("expected no errors after err-guard sibling narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
