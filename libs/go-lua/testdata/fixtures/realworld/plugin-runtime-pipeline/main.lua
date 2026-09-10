local time = require("time")
local result = require("result")
local protocol = require("protocol")
local policy_builder = require("policy_builder")
local plugin_builder = require("plugin_builder")
local helpers = require("helpers")
local runtime = require("runtime")

type StringResult = {ok: true, value: string} | {ok: false, error: result.AppError}

local now = time.now()

local observed_plugin_contents: {[string]: string} = {}
local retry_notes: {string} = {}
local last_runtime_id: string? = nil

local policy = policy_builder.new()
    :named("aggressive")
    :max_attempts(3)
    :scale_by(0.5)
    :retry_on("busy")
    :retry_on("rate_limited")
    :with_backoff(function(attempt: integer): number
        return attempt + 0.25
    end)
    :build()

local search_handler = plugin_builder.new()
    :named("search")
    :arg("query")
    :prefix_with("search")
    :remember_when_flag("warm")
    :decorate(function(content: string, _state: protocol.RuntimeState, call: protocol.PluginCall): string
        return content .. ":" .. helpers.source_tag(call)
    end)
    :tag_with(function(policy_value: protocol.RetryPolicy, _state: protocol.RuntimeState, call: protocol.PluginCall): {[string]: string}
        return {
            source = helpers.source_tag(call),
            policy = helpers.policy_label(policy_value),
            plugin = call.plugin,
        }
    end)
    :build()

local profile_handler = plugin_builder.new()
    :named("profile")
    :arg("user_id")
    :prefix_with("profile")
    :remember_when_flag("cache_hit")
    :decorate(function(content: string, state: protocol.RuntimeState, _call: protocol.PluginCall): string
        local seen = state.last_seen
        if seen then
            return content .. ":repeat"
        end
        return content .. ":first"
    end)
    :tag_with(function(policy_value: protocol.RetryPolicy, _state: protocol.RuntimeState, call: protocol.PluginCall): {[string]: string}
        return {
            source = helpers.source_tag(call),
            policy = policy_value.label,
            plugin = call.plugin,
        }
    end)
    :build()

local app = runtime.new(policy)
    :register_plugin("search", search_handler)
    :register_plugin("profile", profile_handler)

app:on_step(function(step: protocol.RuntimeStep, state: protocol.RuntimeState)
    last_runtime_id = state.id

    if step.kind == "plugin" then
        observed_plugin_contents[step.plugin] = step.output.content
        local cached: boolean = step.output.cached
    elseif step.kind == "hook" then
        table.insert(retry_notes, step.detail)
    else
        local note: string = step.note
        local at_seconds: integer = step.at:unix()
    end
end)

local heartbeat: protocol.HeartbeatEvent = {
    kind = "heartbeat",
    at = now,
}

local search_call: protocol.PluginCall = {
    kind = "plugin_call",
    id = "e1",
    plugin = "search",
    input = {query = "lua"},
    meta = protocol.meta("trace-1", {source = "planner"}),
}

local profile_call: protocol.PluginCall = {
    kind = "plugin_call",
    id = "e2",
    plugin = "profile",
    input = {user_id = "u-1"},
    meta = protocol.meta("trace-2", nil),
}

local repeat_search_call: protocol.PluginCall = {
    kind = "plugin_call",
    id = "e3",
    plugin = "search",
    input = {query = "lua"},
    meta = protocol.meta("trace-3", {source = "planner"}),
}

local done_event: protocol.DoneEvent = {
    kind = "done",
    status = "ok",
    reason = "complete",
    meta = protocol.meta("trace-4", nil),
}

local events: {protocol.Event} = {
    heartbeat,
    search_call,
    profile_call,
    repeat_search_call,
    done_event,
}

local store = app:new_store("runtime-1", now)
store:set_flag("warm")

local summary_result = app:run(store, events, now)
local observed_error_message: string? = nil
summary_result = result.tap_error(summary_result, function(err: result.AppError)
    observed_error_message = err.message
end)

if not summary_result.ok then
    local message: string = summary_result.error.message
    local retryable: boolean = summary_result.error.retryable
else
    local summary = summary_result.value
    local runtime_id: string = summary.id
    local total_steps: number = summary.total_steps
    local cached_count: number = summary.cached_count
    local elapsed_seconds: number = summary.elapsed_seconds
    local last_status: string? = summary.last_status
    local last_plugin: string? = summary.last_plugin
end

local label_result = result.map(summary_result, function(summary: protocol.RuntimeSummary): string
    return summary.id .. ":" .. helpers.status_name(summary.last_status)
end)

if label_result.ok then
    local label: string = label_result.value
end

local last_plugin_result = result.and_then(summary_result, function(summary: protocol.RuntimeSummary): StringResult
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

    local last_plugin = summary.last_plugin or "none"
    return {
        ok = true,
        value = last_plugin,
    }
end)

if last_plugin_result.ok then
    local plugin_name: string = last_plugin_result.value
end

local cached_search = store:lookup("search")
if cached_search then
    local cached_content: string = cached_search.content
    local cached_flag: boolean = cached_search.cached
    local tags = cached_search.tags
    if tags then
        local source = tags["source"]
        if source then
            local stable_source: string = source
        end
    end
end

for plugin_name, content in pairs(observed_plugin_contents) do
    local stable_plugin: string = plugin_name
    local stable_content: string = content
end

for _, detail in ipairs(retry_notes) do
    local retry_note: string = detail
end

if last_runtime_id ~= nil then
    local stable_runtime_id: string = last_runtime_id
end

local search_tags = search_call.meta.tags
if search_tags then
    local source = search_tags["source"]
    if source then
        local source_name: string = source
    end
end

local last_seen = store.state.last_seen or store.state.started_at
local elapsed = now:sub(last_seen)
local seconds: number = elapsed:seconds()
