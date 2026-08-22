package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestSelectorStyleToStringCallOnOptionalInputs(t *testing.T) {
	source := `
		type SelectionResult = {
			success: boolean,
			agent: string,
			reason: string,
		}

		local agent_selector = {}

		function agent_selector.select_agent(user_prompt, class_name): (SelectionResult?, string?)
			if not user_prompt or user_prompt == "" then
				return nil, "User prompt is required"
			end
			if not class_name or class_name == "" then
				return nil, "Class name is required"
			end
			return { success = true, agent = "a", reason = "ok" }, nil
		end

		function agent_selector.execute(input): (SelectionResult?, string?)
			if not input then
				return nil, "Input is required"
			end

			local user_prompt = input.user_prompt or input.prompt
			local class_name = input.class_name or input.class

			return agent_selector.select_agent(tostring(user_prompt), tostring(class_name))
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no checker errors, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
