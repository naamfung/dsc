package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestRegression_NegativeIntegerLiteralInStringSub(t *testing.T) {
	source := `
local archive = "abcdef"
local tail = archive:sub(-2)
local trimmed = archive:sub(1, -2)

local _: string = tail
local __: string = trimmed
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors for negative integer literals in string.sub")
	}
}

func TestRegression_NegativeIntegerLiteralPassesIntegerParam(t *testing.T) {
	source := `
local function takes_integer(v: integer): integer
	return v
end

local _: integer = takes_integer(-1024)
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected unary minus on integer literal to type as integer")
	}
}
