package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

func TestInference_NoFalseNonConvergence_ForMapArrayMutation(t *testing.T) {
	code := `
		local function f(session_data: {meta: any?})
			local current_meta = session_data.meta or {}
			if not current_meta.checkpoints or type(current_meta.checkpoints) ~= "table" then
				current_meta.checkpoints = {}
			end
			table.insert(current_meta.checkpoints, { checkpoint_id = "x" })
			return current_meta
		end
	`

	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no type errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityWarning && strings.Contains(d.Message, "type inference did not converge") {
			t.Fatalf("unexpected non-convergence warning: %v", d.Message)
		}
	}
}

func TestInference_NoFalseNonConvergence_ForBootloaderRequiresNormalization(t *testing.T) {
	code := `
		local function check_dependencies(entry: any, completed_bootloaders: {string}): (boolean, {string}?)
			if not entry.meta or not entry.meta.requires then
				return true, nil
			end

			local requires = entry.meta.requires
			if type(requires) ~= "table" then
				requires = { requires }
			end

			local missing_bootloaders = {}
			for _, dep_id in ipairs(requires) do
				local dep: string = tostring(dep_id)
				local found = false
				for _, completed_id in ipairs(completed_bootloaders) do
					if completed_id == dep then
						found = true
						break
					end
				end
				if not found then
					table.insert(missing_bootloaders, dep)
				end
			end

			if #missing_bootloaders > 0 then
				return false, missing_bootloaders
			end
			return true, nil
		end
	`

	result := testutil.Check(code, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityWarning && strings.Contains(d.Message, "type inference did not converge") {
			t.Fatalf("unexpected non-convergence warning: %v", d.Message)
		}
	}
}
