local time = require("time")
local result = require("result")

type AppError = result.AppError

type RequestMeta = {
    request_id: string,
    trace_id: string,
    tags: {[string]: string}?,
}

type ToolCallMessage = {
    kind: "tool_call",
    id: string,
    tool: string,
    arguments: {[string]: any},
    meta: RequestMeta,
}

type ToolResult = {
    tool: string,
    content: string,
    cached: boolean,
}

type SessionState = {
    id: string,
    started_at: time.Time,
    last_activity: time.Time?,
    tool_cache: {[string]: ToolResult},
}

type ToolResultResult = {ok: true, value: ToolResult} | {ok: false, error: AppError}
type ToolHandler = (SessionState, ToolCallMessage) -> ToolResultResult

local M = {}
M.AppError = AppError
M.RequestMeta = RequestMeta
M.ToolCallMessage = ToolCallMessage
M.ToolResult = ToolResult
M.SessionState = SessionState
M.ToolResultResult = ToolResultResult
M.ToolHandler = ToolHandler

function M.meta(request_id: string, trace_id: string, tags: {[string]: string}?): RequestMeta
    return {
        request_id = request_id,
        trace_id = trace_id,
        tags = tags,
    }
end

return M
