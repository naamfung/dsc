local time = require("time")
local result = require("result")

type AppError = result.AppError

type Usage = {
    prompt_tokens: integer,
    completion_tokens: integer,
}

type RequestMeta = {
    request_id: string,
    trace_id: string,
    tags: {[string]: string}?,
}

type UserMessage = {
    kind: "user",
    id: string,
    content: string,
    meta: RequestMeta,
}

type ToolCallMessage = {
    kind: "tool_call",
    id: string,
    tool: string,
    arguments: {[string]: any},
    meta: RequestMeta,
}

type DoneMessage = {
    kind: "done",
    id: string,
    reason: "complete" | "tool_error" | "timeout",
    usage: Usage?,
    meta: RequestMeta,
}

type Message = UserMessage | ToolCallMessage | DoneMessage

type ToolResult = {
    tool: string,
    content: string,
    cached: boolean,
}

type AssistantStep = {
    kind: "assistant",
    content: string,
}

type ToolStep = {
    kind: "tool",
    tool: string,
    result: ToolResult,
}

type AuditStep = {
    kind: "audit",
    note: string,
    at: time.Time,
}

type WorkflowStep = AssistantStep | ToolStep | AuditStep

type SessionState = {
    id: string,
    started_at: time.Time,
    last_activity: time.Time?,
    messages: {Message},
    steps: {WorkflowStep},
    flags: {[string]: boolean},
    tool_cache: {[string]: ToolResult},
}

type SessionSummary = {
    id: string,
    total_steps: number,
    cached_tool_count: number,
    last_latency_seconds: number,
    last_reason: string?,
}

type StepListener = (WorkflowStep, SessionState) -> ()
type ToolResultResult = {ok: true, value: ToolResult} | {ok: false, error: AppError}
type ToolHandler = (SessionState, ToolCallMessage) -> ToolResultResult

local M = {}
M.AppError = AppError
M.Usage = Usage
M.RequestMeta = RequestMeta
M.UserMessage = UserMessage
M.ToolCallMessage = ToolCallMessage
M.DoneMessage = DoneMessage
M.Message = Message
M.ToolResult = ToolResult
M.WorkflowStep = WorkflowStep
M.SessionState = SessionState
M.SessionSummary = SessionSummary
M.StepListener = StepListener
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
