local result = require("result")
local protocol = require("protocol")

type ToolResultResult = protocol.ToolResultResult

local M = {}

function M.search(_state: protocol.SessionState, msg: protocol.ToolCallMessage): ToolResultResult
    return result.ok({
        tool = msg.tool,
        content = "search",
        cached = false,
    })
end

return M
