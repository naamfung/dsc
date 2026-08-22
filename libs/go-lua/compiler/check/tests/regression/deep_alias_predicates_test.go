package regression

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func aliasChain(prefix, base string, depth int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "type %s0 = %s\n", prefix, base)
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&b, "type %s%d = %s%d\n", prefix, i, prefix, i-1)
	}
	return b.String()
}

func TestDeepAliasPredicates_Arithmetic(t *testing.T) {
	code := aliasChain("N", "number", 10) + `
		local function add1(x: N10): number
			return x + 1
		end
		local y: number = add1(3)
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for deep numeric alias chain, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDeepAliasPredicates_OrFallbackNarrowing(t *testing.T) {
	code := aliasChain("N", "number?", 10) + `
		local x: N10 = nil
		local y: number = x or 1
	`
	result := testutil.Check(code, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for deep optional alias fallback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
