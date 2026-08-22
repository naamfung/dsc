local protocol = require("protocol")
local builder = require("builder")

type Message = {kind: "user", content: string} | protocol.ToolCallMessage

local function run(handlers: {[string]: protocol.ToolHandler}, state: protocol.SessionState, msg: Message)
    if msg.kind ~= "tool_call" then
        return nil
    end

    local handler = handlers[msg.tool]
    if not handler then
        return nil
    end

    local out = handler(state, msg)
    if out.ok then
        local tool: string = out.value.tool
        local content: string = out.value.content
    else
        local code: string = out.error.code
        local retryable: boolean = out.error.retryable
    end
end

run({search = builder.build()}, {flags = {}}, {kind = "tool_call", tool = "search", arguments = {value = "x"}})
