package hooks_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/hooks"
)

// TestHooksCompile is a compile-time guard ensuring hooks build correctly.
func TestHooksCompile(t *testing.T) {
	_ = hooks.CheckCalls
	_ = hooks.CheckFields
	_ = hooks.CheckReturns
	_ = hooks.CheckAssignments
}
