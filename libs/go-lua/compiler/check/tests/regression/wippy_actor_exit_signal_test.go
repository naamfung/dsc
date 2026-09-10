package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

// Regression for wippy actor false positive:
// "cannot index type nil" after is_exit(exit_result) guard.
func TestWippyActor_ExitSignalGuard_NoNilIndexError(t *testing.T) {
	source := `
		type ExitSignal = {
			_actor_exit: boolean,
			result: any,
		}

		local function is_exit(result: any): boolean
			return type(result) == "table" and result._actor_exit == true
		end

		local function is_next(result: any): boolean
			return type(result) == "table" and result._actor_next == true
		end

		local function run_topic(handlers: any)
			local topic_handlers: {[string]: (any, any, any, any) -> any} = {}
			for name, handler in pairs(handlers) do
				if type(handler) == "function" and not name:match("^__") then
					topic_handlers[name] = handler
				end
			end
			local function process_topic_message(state, topic, payload, from)
				local current_topic = topic
				local current_payload = payload
				while true do
					local handler = topic_handlers[current_topic]
					if not handler and current_topic ~= "__default" then
						handler = handlers.__default
					end
					if not handler then
						return nil
					end
						local reply = handler(state, current_payload, current_topic, from)
					if is_next(reply) then
						local next_topic = reply.topic
						if reply.payload ~= nil then
							current_payload = reply.payload
						end
						if not next_topic then
							if handlers.__default then
								current_topic = "__default"
							else
								return nil
							end
						else
							current_topic = next_topic
						end
					else
						return reply
					end
				end
			end

				local state = {}
				local msg = {
					topic = function() return "t" end,
					payload = function() return { data = function() return {} end } end,
					from = function() return "x" end,
				}
				local exit_result = process_topic_message(state, msg:topic(), msg:payload():data(), msg:from())
				if is_exit(exit_result) then
					return exit_result.result
				end
				return nil
			end

		return { run_topic = run_topic }
	`

	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Fatalf("unexpected error at %d:%d: %s", d.Position.Line, d.Position.Column, d.Message)
		}
	}
}
