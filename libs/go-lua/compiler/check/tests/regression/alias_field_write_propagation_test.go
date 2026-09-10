package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: when a table path aliases another symbol (box.cur = s),
// subfield writes through the alias path must update the source symbol shape.
func TestAliasFieldWrite_PropagatesToSourceSymbol(t *testing.T) {
	source := `
		local s = { hook = nil }
		local box = { cur = nil }
		box.cur = s
		box.cur.hook = function() end
		if s.hook then
			s.hook()
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
