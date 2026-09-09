package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Repro from docker-demo: parse_stream_lines + assert.eq(#result, 1) followed by
// result[1].stream should not report "cannot index type nil".
func TestDockerLikeParseStreamLines_IndexAfterEqLen(t *testing.T) {
	source := `
		local json = require("json")

		local test = {}
		function test.eq(actual: any, expected: any, msg: string?)
			if actual ~= expected then
				error(msg or "assertion failed")
			end
		end

		local function parse_stream_lines(raw: any)
			if not raw then
				return {}
			end
			local data = tostring(raw)
			local lines = {}
			local pos = 1
			while pos <= #data do
				local nl = data:find("\n", pos, true)
				local line_end = nl and (nl - 1) or #data
				local line = data:sub(pos, line_end)
				if line ~= "" then
					local ok, parsed = pcall(json.decode, line)
					if ok and parsed then
						table.insert(lines, parsed)
					end
				end
				if not nl then break end
				pos = nl + 1
			end
			return lines
		end

		local result = parse_stream_lines("{\"stream\":\"ok\"}\n")
		test.eq(#result, 1, "one row")
		local stream = result[1].stream
		return stream
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
