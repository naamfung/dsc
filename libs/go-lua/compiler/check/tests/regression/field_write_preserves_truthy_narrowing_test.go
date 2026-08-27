package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: truthy narrowing must survive SSA version bumps caused by
// field writes on the narrowed value.
func TestFieldWritePreservesTruthyNarrowingAcrossVersions(t *testing.T) {
	source := `
		type PluginState = {
			pid: string?,
			restart_count: number,
		}

		local active: {[string]: PluginState} = {}
		local core_prefix: string? = nil
		local core_state: PluginState? = nil

		for prefix, state in pairs(active) do
			core_prefix = prefix
			core_state = state
			break
		end

		if not core_prefix or not core_state then
			return
		end

		core_state.pid = nil
		core_state.restart_count = core_state.restart_count + 1
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
