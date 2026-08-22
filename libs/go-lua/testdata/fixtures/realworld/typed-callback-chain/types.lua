type ToolCall = {
    id: string,
    name: string,
    arguments: {[string]: any},
}

type Usage = {
    input_tokens: number,
    output_tokens: number,
}

type StreamResult = {
    content: string,
    tool_calls: {ToolCall},
    finish_reason: string?,
    usage: Usage,
}

type ErrorInfo = {
    message: string,
    code: string?,
}

type StreamCallbacks = {
    on_content: ((chunk: string) -> ())?,
    on_tool_call: ((call: ToolCall) -> ())?,
    on_error: ((err: ErrorInfo) -> ())?,
    on_done: ((result: StreamResult) -> ())?,
}

local M = {}
M.ToolCall = ToolCall
M.StreamResult = StreamResult
M.ErrorInfo = ErrorInfo
M.StreamCallbacks = StreamCallbacks

function M.empty_result(): StreamResult
    local empty_tools: {ToolCall} = {}
    local empty_usage: Usage = {input_tokens = 0, output_tokens = 0}
    local result: StreamResult = {
        content = "",
        tool_calls = empty_tools,
        finish_reason = nil,
        usage = empty_usage,
    }
    return result
end

return M
