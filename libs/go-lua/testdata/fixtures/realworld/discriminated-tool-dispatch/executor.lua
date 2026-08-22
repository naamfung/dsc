local tools = require("tools")

local M = {}

function M.execute(tool: tools.Tool): tools.ToolResult
    if tool.type == "search" then
        local query: string = tool.args.query
        local limit: number = tool.args.limit or 10
        return {
            tool_name = tool.name,
            output = "Found " .. tostring(limit) .. " results for: " .. query,
            success = true,
        }
    elseif tool.type == "fetch" then
        local url: string = tool.args.url
        local method: string = tool.args.method or "GET"
        return {
            tool_name = tool.name,
            output = method .. " " .. url .. " -> 200 OK",
            success = true,
        }
    elseif tool.type == "compute" then
        local expr: string = tool.args.expression
        return {
            tool_name = tool.name,
            output = "Result of " .. expr .. " = 42",
            success = true,
        }
    end
    return {tool_name = "unknown", output = "unsupported tool type", success = false}
end

function M.execute_batch(tool_list: {tools.Tool}): {tools.ToolResult}
    local results: {tools.ToolResult} = {}
    for _, tool in ipairs(tool_list) do
        table.insert(results, M.execute(tool))
    end
    return results
end

return M
