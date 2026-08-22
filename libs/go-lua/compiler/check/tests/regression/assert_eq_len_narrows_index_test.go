package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: assertion wrappers that encode equality on return should
// preserve expression-level narrowing (e.g. #arr == 1 implies arr[1] is present).
func TestAssertEqLenNarrowsIndexedRead(t *testing.T) {
	source := `
		type Row = { stream: string }

		local test = {}
		function test.eq(actual: any, expected: any, msg: string?)
			if actual ~= expected then
				error(msg or "assertion failed")
			end
		end

		local function parse_stream_lines(raw: any): {Row}
			local lines = {}
			if raw and type(raw) == "string" and raw ~= "" then
				table.insert(lines, { stream = "ok" })
			end
			return lines
		end

		local result = parse_stream_lines("raw")
		test.eq(#result, 1, "one row")

		local line: string = result[1].stream
		return line
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
