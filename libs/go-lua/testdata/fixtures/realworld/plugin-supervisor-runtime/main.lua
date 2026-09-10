local time = require("time")
local result = require("result")
local protocol = require("protocol")
local helpers = require("helpers")
local validator_builder = require("validator_builder")
local handler_builder = require("handler_builder")
local fallback_builder = require("fallback_builder")
local runtime = require("runtime")

type StringResult = {ok: true, value: string} | {ok: false, error: result.AppError}

local now = time.now()

local observed_outputs: {[string]: string} = {}
local observed_fallbacks: {string} = {}
local observed_audits: {string} = {}
local last_runtime_id: string? = nil

local source_validator = validator_builder.new()
    :named("source")
    :require_tag("source")
    :remember_flag("saw_source")
    :build()

local scope_validator = validator_builder.new()
    :named("scope")
    :require_tag("scope")
    :remember_flag("saw_scope")
    :build()

local render_handler = handler_builder.new()
    :named("render")
    :prefix_with("render")
    :require_tag("source")
    :remember_flag("did_render")
    :decorate(function(label: string, envelope: protocol.PayloadEnvelope, _state: protocol.StoreState): string
        if envelope.payload.kind == "render" then
            return label .. ":" .. envelope.tenant_id
        end
        return label
    end)
    :build()

local index_handler = handler_builder.new()
    :named("index")
    :prefix_with("index")
    :require_tag("scope")
    :remember_flag("did_index")
    :decorate(function(label: string, envelope: protocol.PayloadEnvelope, _state: protocol.StoreState): string
        if envelope.payload.kind == "index" then
            return label .. ":" .. envelope.payload.document_id
        end
        return label
    end)
    :build()

local audit_handler = handler_builder.new()
    :named("audit")
    :prefix_with("audit")
    :remember_flag("did_audit")
    :fail_on_tag("retry", "busy")
    :decorate(function(label: string, envelope: protocol.PayloadEnvelope, _state: protocol.StoreState): string
        if envelope.payload.kind == "audit" then
            return label .. ":" .. envelope.payload.actor_id
        end
        return label
    end)
    :build()

local retry_fallback = fallback_builder.new()
    :for_plugin("audit")
    :retry_on("busy")
    :queue_named("audit-retry")
    :decorate_note(function(note: string, request: protocol.DispatchRequest, err: result.AppError): string
        return note .. ":" .. request.envelope.id .. ":" .. err.code
    end)
    :build()

local app = runtime.new()
    :use_validator(source_validator)
    :use_validator(scope_validator)
    :register_handler("render", render_handler)
    :register_handler("index", index_handler)
    :register_handler("audit", audit_handler)
    :use_fallback(retry_fallback)

app:on_step(function(step: protocol.RuntimeStep, state: protocol.StoreState)
    last_runtime_id = state.id

    if step.kind == "dispatch" then
        observed_outputs[step.plugin] = helpers.output_label(step.output)
    elseif step.kind == "fallback" then
        table.insert(observed_fallbacks, step.note)
        local retry_seconds: integer = step.retry_at:unix()
    else
        table.insert(observed_audits, step.note)
        local at_seconds: integer = step.at:unix()
    end
end)

local render_request: protocol.DispatchRequest = {
    kind = "dispatch",
    plugin = "render",
    envelope = {
        id = "req-1",
        tenant_id = "tenant-a",
        payload = {
            kind = "render",
            template = "welcome",
            values = {subject = "hello"},
        },
        meta = protocol.meta("trace-1", {source = "api", scope = "render"}),
    },
}

local repeat_render_request: protocol.DispatchRequest = {
    kind = "dispatch",
    plugin = "render",
    envelope = {
        id = "req-1",
        tenant_id = "tenant-a",
        payload = {
            kind = "render",
            template = "welcome",
            values = {subject = "hello"},
        },
        meta = protocol.meta("trace-2", {source = "api", scope = "render"}),
    },
}

local index_request: protocol.DispatchRequest = {
    kind = "dispatch",
    plugin = "index",
    envelope = {
        id = "req-2",
        tenant_id = "tenant-a",
        payload = {
            kind = "index",
            document_id = "doc-1",
            terms = {"lua", "types", "cache"},
        },
        meta = protocol.meta("trace-3", {source = "worker", scope = "index"}),
    },
}

local audit_request: protocol.DispatchRequest = {
    kind = "dispatch",
    plugin = "audit",
    envelope = {
        id = "req-3",
        tenant_id = "tenant-b",
        payload = {
            kind = "audit",
            action = "login",
            actor_id = "actor-7",
        },
        meta = protocol.meta("trace-4", {source = "worker", scope = "audit", retry = "true"}),
    },
}

local tick_request: protocol.TickRequest = {
    kind = "tick",
    at = now,
}

local requests: {protocol.Request} = {
    render_request,
    repeat_render_request,
    index_request,
    audit_request,
    tick_request,
}

local store = app:new_store("supervisor-1", now)
local summary_result = app:run(store, requests, now)
if not summary_result.ok then
    local message: string = summary_result.error.message
    local retryable: boolean = summary_result.error.retryable
else
    local summary = summary_result.value
    local runtime_id: string = summary.id
    local processed: number = summary.processed
    local cached_hits: number = summary.cached_hits
    local fallback_count: number = summary.fallback_count
    local elapsed_seconds: time.Duration = summary.elapsed_seconds
    local last_output_kind: string? = summary.last_output_kind
end

local label_result = result.map(summary_result, function(summary: protocol.RunSummary): string
    return summary.id .. ":" .. tostring(summary.cached_hits + summary.fallback_count)
end)

if label_result.ok then
    local label: string = label_result.value
end

local checked_result = result.and_then(summary_result, function(summary: protocol.RunSummary): StringResult
    if summary.processed < 4 then
        return {
            ok = false,
            error = {
                code = "invalid",
                message = "expected processed steps",
                retryable = false,
            },
        }
    end

    return {ok = true, value = summary.id}
end)

if checked_result.ok then
    local checked_id: string = checked_result.value
end

local cache_key: string = helpers.cache_key(render_request)
local cached_render = store:lookup_cached(cache_key)
if cached_render then
    local seen_at: integer = cached_render.seen_at:unix()
    local receipt = cached_render.value
    local plugin_name: string = receipt.plugin
    if receipt.output.kind == "rendered" then
        local rendered: protocol.RenderOutput = receipt.output
        local body: string = rendered.body
        local label = rendered.label
        if label then
            local stable_label: string = label
        end
    end
end

local render_count = store.state.plugin_counts["render"]
if render_count then
    local count: integer = render_count
end

local cached_count = store.state.plugin_counts["cached"]
if cached_count then
    local count: integer = cached_count
end

local fallback_count = store.state.plugin_counts["fallback"]
if fallback_count then
    local count: integer = fallback_count
end

local saw_source = store.state.flags["saw_source"]
if saw_source then
    local flag: boolean = saw_source
end

local saw_fallback = store.state.flags["saw_fallback"]
if saw_fallback then
    local flag: boolean = saw_fallback
end

local last_tick = store.state.last_tick
if last_tick then
    local tick_seconds: integer = last_tick:unix()
end

local last_dispatch = store.state.last_dispatch_at
if last_dispatch then
    local elapsed = now:sub(last_dispatch)
    local seconds: number = elapsed:seconds()
end

local last_audit = store.state.audit_tags["last"]
if last_audit then
    local note: string = last_audit
end

for plugin_name, output in pairs(observed_outputs) do
    local stable_plugin: string = plugin_name
    local stable_output: string = output
end

for _, note in ipairs(observed_fallbacks) do
    local stable_note: string = note
end

for _, note in ipairs(observed_audits) do
    local stable_note: string = note
end

if last_runtime_id then
    local runtime_id: string = last_runtime_id
end
