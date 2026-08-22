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
		local plugin_prefix: string? = nil
		local plugin_state: PluginState? = nil

		for prefix, state in pairs(active) do
			plugin_prefix = prefix
			plugin_state = state
			break
		end

		if not plugin_prefix or not plugin_state then
			return
		end

		plugin_state.pid = nil
		plugin_state.restart_count = plugin_state.restart_count + 1
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
