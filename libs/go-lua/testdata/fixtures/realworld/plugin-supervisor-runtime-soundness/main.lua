local time = require("time")
local protocol = require("protocol")
local validator_builder = require("validator_builder")
local handler_builder = require("handler_builder")
local fallback_builder = require("fallback_builder")
local runtime = require("runtime")

local now = time.now()

local source_validator = validator_builder.new()
    :named("source")
    :require_tag("source")
    :build()

local render_handler = handler_builder.new()
    :named("render")
    :prefix_with("render")
    :remember_flag("did_render")
    :build()

local audit_handler = handler_builder.new()
    :named("audit")
    :prefix_with("audit")
    :fail_on_tag("retry", "busy")
    :build()

local retry_fallback = fallback_builder.new()
    :for_plugin("audit")
    :retry_on("busy")
    :queue_named("audit-retry")
    :build()

local app = runtime.new()
    :use_validator(source_validator)
    :register_handler("render", render_handler)
    :register_handler("audit", audit_handler)
    :use_fallback(retry_fallback)

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
        meta = protocol.meta("trace-1", {source = "api"}),
    },
}

local audit_request: protocol.DispatchRequest = {
    kind = "dispatch",
    plugin = "audit",
    envelope = {
        id = "req-2",
        tenant_id = "tenant-a",
        payload = {
            kind = "audit",
            action = "login",
            actor_id = "actor-7",
        },
        meta = protocol.meta("trace-2", {source = "worker", retry = "true"}),
    },
}

local store = app:new_store("supervisor-1", now)
local _rendered = app:dispatch(store, render_request, now)
local _ = app:dispatch(store, audit_request, now)

local missing_handler = app.handlers["missing"]
local missing_result = missing_handler(store.state, render_request.envelope, now) -- expect-error

local cached_render = store:lookup_cached("render:req-1")
if cached_render then
    local seen_seconds: integer = cached_render.seen_at -- expect-error
    local cached_flag: boolean = store.state.flags["did_index"] -- expect-error
end
local fallback_total: integer = store.state.plugin_counts["fallback"] -- expect-error
local last_audit: string = store.state.audit_tags["last"] -- expect-error
local elapsed = now:sub(store.state.last_tick) -- expect-error
