package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestFP_StringUnpackLiteralFormatsProduceIntegers(t *testing.T) {
	source := `
		local function parse(buf: string)
			local total_length: integer = string.unpack(">I4", buf, 1)
			local fmt = ">I4"
			local headers_length: integer = string.unpack(fmt, buf, 5)

			local payload_offset: integer = 13 + headers_length
			local payload_length: integer = total_length - 12 - headers_length - 4
			local payload = buf:sub(payload_offset, payload_offset + payload_length - 1)
			return payload
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for string.unpack literal formats, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
