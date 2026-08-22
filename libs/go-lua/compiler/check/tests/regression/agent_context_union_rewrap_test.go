package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestAgentStyleUnionRewrapAssignment(t *testing.T) {
	source := `
		type ToolSpec = string | { id: string, alias: string? }

		local M = {}

		function M:add_tools(tool_specs: ToolSpec | ToolSpec[])
			if not tool_specs then
				return self
			end

			if type(tool_specs) == "string" or (type(tool_specs) == "table" and tool_specs.id) then
				local _single: ToolSpec = tool_specs
				tool_specs = { tool_specs }
			end

			for _, tool_spec in ipairs(tool_specs) do
				if type(tool_spec) == "string" then
					local _ = tool_spec
				else
					local _ = tool_spec.id
				end
			end

			return self
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no checker errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestAgentStyleUnionRewrapAssignmentInlineUnion(t *testing.T) {
	source := `
		type Elem = string | { id: string, alias: string? }

		local M = {}

		function M:add_tools(tool_specs: Elem | Elem[])
			if not tool_specs then
				return self
			end

			if type(tool_specs) == "string" or (type(tool_specs) == "table" and tool_specs.id) then
				tool_specs = { tool_specs }
			end

			for _, tool_spec in ipairs(tool_specs) do
				if type(tool_spec) == "string" then
					local _ = tool_spec
				else
					local _ = tool_spec.id
				end
			end

			return self
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no checker errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
