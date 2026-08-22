local time = require("time")
local result = require("result")

type AppError = result.AppError

type RetryPolicy = {
    label: string,
    max_attempts: integer,
    compute_delay: (integer) -> number,
    should_retry: (AppError, integer) -> boolean,
}

type RequestMeta = {
    trace_id: string,
    tags: {[string]: string}?,
}

type PluginCall = {
    kind: "plugin_call",
    id: string,
    plugin: string,
    input: {[string]: any},
    meta: RequestMeta,
}

type HeartbeatEvent = {
    kind: "heartbeat",
    at: time.Time,
}

type DoneEvent = {
    kind: "done",
    status: "ok" | "failed",
    reason: string?,
    meta: RequestMeta,
}

type Event = PluginCall | HeartbeatEvent | DoneEvent

type PluginOutput = {
    plugin: string,
    content: string,
    cached: boolean,
    tags: {[string]: string}?,
}

type PluginStep = {
    kind: "plugin",
    plugin: string,
    output: PluginOutput,
}

type AuditStep = {
    kind: "audit",
    note: string,
    at: time.Time,
}

type HookStep = {
    kind: "hook",
    name: string,
    detail: string,
}

type RuntimeStep = PluginStep | AuditStep | HookStep

type RuntimeState = {
    id: string,
    started_at: time.Time,
    last_seen: time.Time?,
    steps: {RuntimeStep},
    cache: {[string]: PluginOutput},
    flags: {[string]: boolean},
}

type RuntimeSummary = {
    id: string,
    total_steps: number,
    cached_count: number,
    last_status: string?,
    elapsed_seconds: number,
    last_plugin: string?,
}

type Hook = (RuntimeStep, RuntimeState) -> ()
type PluginResult = {ok: true, value: PluginOutput} | {ok: false, error: AppError}
type RuntimeResult = {ok: true, value: RuntimeSummary} | {ok: false, error: AppError}
type PluginHandler = (RuntimeState, PluginCall, RetryPolicy) -> PluginResult

local M = {}
M.AppError = AppError
M.RetryPolicy = RetryPolicy
M.RequestMeta = RequestMeta
M.PluginCall = PluginCall
M.HeartbeatEvent = HeartbeatEvent
M.DoneEvent = DoneEvent
M.Event = Event
M.PluginOutput = PluginOutput
M.RuntimeStep = RuntimeStep
M.RuntimeState = RuntimeState
M.RuntimeSummary = RuntimeSummary
M.Hook = Hook
M.PluginResult = PluginResult
M.RuntimeResult = RuntimeResult
M.PluginHandler = PluginHandler

function M.meta(trace_id: string, tags: {[string]: string}?): RequestMeta
    return {
        trace_id = trace_id,
        tags = tags,
    }
end

return M
