package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestAny_MethodCallIsAllowedButAssignmentRemainsChecked(t *testing.T) {
	source := `
		local x: any = {}
		local v = x:get_full_context()
		local y: string = v
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected assignment error from any to string")
	}
}
