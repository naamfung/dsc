package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Explicit `any` parameter annotations are contracts and must not be rewritten
// by call-site param hints. This mirrors wippy.test:runner wait_for(ch: any).
func TestExplicitAnyParamAnnotation_IsNotRewrittenByHints(t *testing.T) {
	source := `
		local function wait_for(ch: any, timeout: any)
			return ch
		end

		local function run()
			local n: number = 1
			wait_for(n, "1s")

			local cmd: any = {}
			wait_for(cmd:response(), "30s")
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Fatalf("expected no errors; explicit any param annotation must remain any")
	}
}
