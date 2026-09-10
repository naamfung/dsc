package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard for loop-field phi merging:
// while not self.stopping with branch-local self.stopping = true must not poison
// sibling fields (self.pending_ops, self.context.queue_empty_callback) to never.
func TestChannelSelectSelfFieldNarrowing_DoesNotPoisonSiblingFields(t *testing.T) {
	source := `
		local Bus = {
			ops_channel = nil :: any,
			stop_signal = nil :: any,
			stopping = false,
			pending_ops = 0 :: number,
			context = nil :: any,
		}
		Bus.__index = Bus

		function Bus.new()
			local self = setmetatable({}, Bus)
			self.ops_channel = channel.new(1)
			self.stop_signal = channel.new(1)
			self.pending_ops = 1
			self.context = {
				queue_empty_callback = function()
				end,
			}
			return self
		end

		function Bus:run()
			while not self.stopping do
				local result = channel.select({
					self.stop_signal:case_receive(),
					self.ops_channel:case_receive()
				})

				if not result.ok then
					break
				end

				if result.channel == self.stop_signal then
					self.stopping = true
				elseif result.channel == self.ops_channel then
					self.pending_ops = self.pending_ops - 1
					if self.pending_ops == 0 then
						if self.context.queue_empty_callback then
							self.context.queue_empty_callback()
						end
					end
				end
			end
		end

		local b = Bus.new()
		b:run()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
