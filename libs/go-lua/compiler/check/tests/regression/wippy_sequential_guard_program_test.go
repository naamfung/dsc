package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Program-level regressions mirroring wippy lint patterns.
func TestWippySequentialGuards_ErrorKindProgram(t *testing.T) {
	source := `
		local M = {}
		type Err = {kind: string}

		function M.error_kind(err: Err?, expected_kind: string, msg: string?)
			if err == nil then error("nil") end
			if type(err) ~= "table" then error("not table") end
			if err.kind ~= expected_kind then
				error(msg or "wrong kind")
			end
			local k: string = err.kind
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for wippy error_kind pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestWippySequentialGuards_ActorMetaProgram(t *testing.T) {
	source := `
		type Meta = {role: string, department: string}
		type Actor = {meta: (self: Actor) -> Meta?}

		function validate(actor: Actor)
			local meta = actor:meta()
			if not meta then error("no meta") end
			if meta.role ~= "admin" then error("not admin") end
			if meta.department ~= "engineering" then error("not engineering") end
			local dept: string = meta.department
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for wippy actor meta guard pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
