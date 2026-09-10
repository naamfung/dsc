package regression

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

func numericAliasChain(depth int) string {
	var b strings.Builder
	b.WriteString("type N0 = number\n")
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&b, "type N%d = N%d\n", i, i-1)
	}
	return b.String()
}

func TestParamHints_DeepAliasChain_NoInterprocNonConvergenceWarning(t *testing.T) {
	code := numericAliasChain(32) + `
		local function g(x)
			return x + 1
		end

		local function f(v: N32): number
			return g(v)
		end

		local n: number = f(1)
	`

	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityWarning && strings.Contains(d.Message, "inter-function fixpoint did not converge") {
			t.Fatalf("unexpected non-convergence warning: %v", d.Message)
		}
	}
}
