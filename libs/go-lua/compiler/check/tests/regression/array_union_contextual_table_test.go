package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestArrayUnionContextualTyping_UsesExpectedElementType(t *testing.T) {
	result := testutil.Check(`
type ContentEvent = {type: "content", data: string}
type ToolCallEvent = {type: "tool_call", id: string, name: string, arguments: {[string]: any}}
type DoneEvent = {type: "done", reason: string?, usage: {input_tokens: number, output_tokens: number}?}
type StreamEvent = ContentEvent | ToolCallEvent | DoneEvent

local events: {StreamEvent} = {
    {type = "content", data = "Hello"},
    {type = "tool_call", id = "t1", name = "search", arguments = {query = "test"}},
}
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for union array element contextual typing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
