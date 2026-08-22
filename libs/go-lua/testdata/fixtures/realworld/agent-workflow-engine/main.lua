local time = require("time")
local result = require("result")
local engine = require("engine")
local tool_builder = require("tool_builder")
local tools = require("tools")
local protocol = require("protocol")

type StringResult = {ok: true, value: string} | {ok: false, error: result.AppError}

local now = time.now()

local observed_steps: {protocol.WorkflowStep} = {}
local observed_tool_contents: {[string]: string} = {}
local last_session_id: string? = nil

local search_handler = tool_builder.new()
    :named("search")
    :require_arg("topic")
    :prefix_with("search")
    :remember_flag("warm_cache")
    :with_formatter(function(content: string, _state: protocol.SessionState, msg: protocol.ToolCallMessage): string
        return content .. ":" .. tools.source_tag(msg)
    end)
    :build()

local profile_handler = tool_builder.new()
    :named("profile")
    :require_arg("user_id")
    :prefix_with("profile")
    :remember_flag("profile_loaded")
    :with_formatter(function(content: string, state: protocol.SessionState, _msg: protocol.ToolCallMessage): string
        return content .. ":" .. tools.cache_mode(state)
    end)
    :build()

local app = engine.new()
    :register_tool("search", search_handler)
    :register_tool("profile", profile_handler)

app:on_step(function(step: protocol.WorkflowStep, state: protocol.SessionState)
    last_session_id = state.id

    if step.kind == "assistant" then
        local text: string = step.content
    elseif step.kind == "tool" then
        observed_tool_contents[step.tool] = step.result.content
        local cached: boolean = step.result.cached
    else
        local note: string = step.note
        local at: integer = step.at:unix()
    end

    table.insert(observed_steps, step)
end)

local user_msg: protocol.UserMessage = {
    kind = "user",
    id = "m1",
    content = "hello",
    meta = protocol.meta("req-1", "trace-1", {source = "ui"}),
}

local search_msg: protocol.ToolCallMessage = {
    kind = "tool_call",
    id = "m2",
    tool = "search",
    arguments = {topic = "lua"},
    meta = protocol.meta("req-2", "trace-1", {source = "planner"}),
}

local profile_msg: protocol.ToolCallMessage = {
    kind = "tool_call",
    id = "m3",
    tool = "profile",
    arguments = {user_id = "u-1"},
    meta = protocol.meta("req-3", "trace-1", nil),
}

local repeat_search_msg: protocol.ToolCallMessage = {
    kind = "tool_call",
    id = "m4",
    tool = "search",
    arguments = {topic = "lua"},
    meta = protocol.meta("req-4", "trace-1", {source = "planner"}),
}

local done_msg: protocol.DoneMessage = {
    kind = "done",
    id = "m5",
    reason = "complete",
    usage = {prompt_tokens = 12, completion_tokens = 5},
    meta = protocol.meta("req-5", "trace-1", nil),
}

local messages: {protocol.Message} = {
    user_msg,
    search_msg,
    profile_msg,
    repeat_search_msg,
    done_msg,
}

local store = app:new_session("sess-1", now)
store:mark_flag("warm_cache")

local summary_result = app:process(store, messages, now)
if not summary_result.ok then
    local message: string = summary_result.error.message
    local retryable: boolean = summary_result.error.retryable
else
    local summary = summary_result.value
    local session_id: string = summary.id
    local total_steps: number = summary.total_steps
    local cached_tool_count: number = summary.cached_tool_count
    local latency: number = summary.last_latency_seconds
    local reason: string? = summary.last_reason
end

local summary_label = result.map(summary_result, function(summary: protocol.SessionSummary): string
    return summary.id .. ":" .. tostring(summary.total_steps)
end)

if summary_label.ok then
    local label: string = summary_label.value
end

local summary_id = result.and_then(summary_result, function(summary: protocol.SessionSummary): StringResult
    if summary.total_steps == 0 then
        return {
            ok = false,
            error = {
                code = "invalid",
                message = "expected steps",
                retryable = false,
            },
        }
    end
    return {
        ok = true,
        value = summary.id,
    }
end)

if summary_id.ok then
    local id: string = summary_id.value
end

local cached = store:lookup_tool("search")
if cached then
    local content: string = cached.content
    local from_cache: boolean = cached.cached
end

local missing = store:lookup_tool("missing")
if missing == nil then
    local fallback: string = "missing"
end

for name, content in pairs(observed_tool_contents) do
    local tool_name: string = name
    local tool_content: string = content
end

if last_session_id ~= nil then
    local stable_id: string = last_session_id
end

local search_tags = search_msg.meta.tags
if search_tags then
    local source = search_tags["source"]
    if source then
        local source_name: string = source
    end
end

local last_seen = store.state.last_activity or store.state.started_at
local elapsed = now:sub(last_seen)
local seconds: number = elapsed:seconds()
