package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Literal unions in record fields should subtype their primitive field type
// requirements (e.g. 0|8000 <: integer, ""| "x" <: string).
func TestLiteralUnionRecordField_SubtypesPrimitiveField(t *testing.T) {
	source := `
		type ModelInfo = {
			max_context_tokens: integer,
			name: string,
		}

		local function consume(info: ModelInfo)
			return info
		end

		local max_context_tokens: 0 | 8000
		if true then
			max_context_tokens = 0
		else
			max_context_tokens = 8000
		end

		local name: "" | "alpha"
		if true then
			name = ""
		else
			name = "alpha"
		end

		local info = {
			max_context_tokens = max_context_tokens,
			name = name,
		}

		local _ = consume(info)
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
