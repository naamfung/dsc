type AppError = {
    code: string,
    message: string,
    retryable: boolean,
}

type ToolCallMessage = {
    kind: "tool_call",
    tool: string,
    arguments: {[string]: any},
}

type ToolResult = {
    tool: string,
    content: string,
    cached: boolean,
}

type SessionState = {
    flags: {[string]: boolean},
}

type ToolResultResult = {ok: true, value: ToolResult} | {ok: false, error: AppError}
type ToolHandler = (SessionState, ToolCallMessage) -> ToolResultResult

local M = {}
M.AppError = AppError
M.ToolCallMessage = ToolCallMessage
M.ToolResult = ToolResult
M.SessionState = SessionState
M.ToolResultResult = ToolResultResult
M.ToolHandler = ToolHandler

return M
