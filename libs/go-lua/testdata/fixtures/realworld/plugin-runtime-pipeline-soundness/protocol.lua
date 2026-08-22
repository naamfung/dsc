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

type PluginOutput = {
    plugin: string,
    content: string,
    cached: boolean,
    tags: {[string]: string}?,
}

type RuntimeStep = {
    kind: "hook",
    name: string,
    detail: string,
}

type RuntimeState = {
    id: string,
    started_at: time.Time,
    last_seen: time.Time?,
    steps: {RuntimeStep},
    cache: {[string]: PluginOutput},
    flags: {[string]: boolean},
}

type Hook = (RuntimeStep, RuntimeState) -> ()
type PluginResult = {ok: true, value: PluginOutput} | {ok: false, error: AppError}
type PluginHandler = (RuntimeState, PluginCall, RetryPolicy) -> PluginResult

local M = {}
M.AppError = AppError
M.RetryPolicy = RetryPolicy
M.RequestMeta = RequestMeta
M.PluginCall = PluginCall
M.PluginOutput = PluginOutput
M.RuntimeStep = RuntimeStep
M.RuntimeState = RuntimeState
M.Hook = Hook
M.PluginResult = PluginResult
M.PluginHandler = PluginHandler

function M.meta(trace_id: string, tags: {[string]: string}?): RequestMeta
    return {
        trace_id = trace_id,
        tags = tags,
    }
end

return M
