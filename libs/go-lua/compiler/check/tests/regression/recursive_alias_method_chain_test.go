package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Recursive named builder aliases must preserve their receiver type through
// chained method returns.
func TestRecursiveAliasMethodChain(t *testing.T) {
	source := `
		type Builder = {
			f: (self: Builder) -> Builder,
			g: (self: Builder) -> number,
		}

		local b: Builder = {
			f = function(self: Builder): Builder
				return self
			end,
			g = function(self: Builder): number
				return 1
			end,
		}

		local n: number = b:f():g()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
