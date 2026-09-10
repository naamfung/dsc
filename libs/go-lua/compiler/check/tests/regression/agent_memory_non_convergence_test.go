package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

func TestInference_NoFalseNonConvergence_ForAgentMemoryArrays(t *testing.T) {
	code := `
		local function extract_memory_ids_from_messages(messages: any, scan_limit: any): any
			local memory_ids = {}
			for _, message in ipairs(messages) do
				if message.metadata and message.metadata.memory_ids then
					for _, memory_id in ipairs(message.metadata.memory_ids) do
						table.insert(memory_ids, memory_id)
					end
				end
			end
			return memory_ids
		end

		local function f(runtime_options: any, recall_result: any, tools: any, prompt_builder: any, options: any)
			local messages = prompt_builder:get_messages()
			local prompt_memory_ids = extract_memory_ids_from_messages(messages, options.scan_limit)

			local all_previous_memory_ids = table.create(#prompt_memory_ids + 10, 0)
			for _, id in ipairs(prompt_memory_ids) do
				table.insert(all_previous_memory_ids, id)
			end

			if runtime_options and runtime_options.previous_memory_ids then
				for _, id in ipairs(runtime_options.previous_memory_ids) do
					table.insert(all_previous_memory_ids, id)
				end
			end

			local memory_ids = table.create(#recall_result.memories, 0)
			for _, memory in ipairs(recall_result.memories) do
				table.insert(memory_ids, memory.id)
			end

			local schema_count = 0
			for _, tool_info in pairs(tools) do
				if tool_info.schema then
					schema_count = schema_count + 1
				end
			end

			local names = table.create(schema_count, 0)
			for canonical_name, tool_info in pairs(tools) do
				if tool_info.schema then
					table.insert(names, canonical_name)
				end
			end
			table.sort(names)

			return all_previous_memory_ids, memory_ids, names
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
