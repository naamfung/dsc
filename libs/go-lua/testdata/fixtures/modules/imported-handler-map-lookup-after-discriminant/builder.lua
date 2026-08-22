local protocol = require("protocol")

type ToolResultResult = protocol.ToolResultResult

local M = {}

function M.build(): protocol.ToolHandler
    return function(state: protocol.SessionState, msg: protocol.ToolCallMessage): ToolResultResult
        local value = msg.arguments["value"]
        if type(value) ~= "string" then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = "value must be string",
                    retryable = false,
                },
            }
        end

        if state.flags["flagged"] then
            return {
                ok = true,
                value = {
                    tool = msg.tool,
                    content = "flagged:" .. value,
                    cached = false,
                },
            }
        end

        return {
            ok = true,
            value = {
                tool = msg.tool,
                content = value,
                cached = false,
            },
        }
    end
end

return M
