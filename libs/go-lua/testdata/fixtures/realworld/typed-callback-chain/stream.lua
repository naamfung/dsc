local types = require("types")

type ContentEvent = {type: "content", data: string}
type ToolCallEvent = {type: "tool_call", id: string, name: string, arguments: {[string]: any}}
type ErrorEvent = {type: "error", message: string, code: string?}
type DoneEvent = {type: "done", reason: string?, usage: types.Usage?}
type StreamEvent = ContentEvent | ToolCallEvent | ErrorEvent | DoneEvent

local M = {}

function M.process(events: {StreamEvent}, callbacks: types.StreamCallbacks?): (types.StreamResult?, string?)
    callbacks = callbacks or {}

    local on_content = callbacks.on_content
    local on_tool_call = callbacks.on_tool_call
    local on_error = callbacks.on_error
    local on_done = callbacks.on_done

    local content = ""
    local tool_calls: {types.ToolCall} = {}
    local finish_reason: string? = nil
    local usage: types.Usage = {input_tokens = 0, output_tokens = 0}

    for _, event in ipairs(events) do
        if event.type == "content" then
            content = content .. event.data
            if on_content then
                on_content(event.data)
            end
        elseif event.type == "tool_call" then
            local call: types.ToolCall = {
                id = event.id,
                name = event.name,
                arguments = event.arguments,
            }
            table.insert(tool_calls, call)
            if on_tool_call then
                on_tool_call(call)
            end
        elseif event.type == "error" then
            if on_error then
                on_error({message = event.message, code = event.code})
            end
            return nil, event.message
        elseif event.type == "done" then
            finish_reason = event.reason
            if event.usage then
                usage = event.usage
            end
        end
    end

    local result: types.StreamResult = {
        content = content,
        tool_calls = tool_calls,
        finish_reason = finish_reason,
        usage = usage,
    }

    if on_done then
        on_done(result)
    end

    return result, nil
end

return M
