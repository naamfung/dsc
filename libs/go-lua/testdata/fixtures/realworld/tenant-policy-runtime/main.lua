local time = require("time")
local result = require("result")
local protocol = require("protocol")
local helpers = require("helpers")
local validator_builder = require("validator_builder")
local rule_builder = require("rule_builder")
local runtime = require("runtime")

type StringResult = {ok: true, value: string} | {ok: false, error: result.AppError}

local now = time.now()

local observed_decisions: {[string]: string} = {}
local observed_defers: {string} = {}
local observed_audits: {string} = {}
local last_runtime_id: string? = nil

local source_validator = validator_builder.new()
    :named("source")
    :require_tag("source")
    :remember_flag("saw_source")
    :build()

local auth_validator = validator_builder.new()
    :named("auth")
    :require_scope("policy.read")
    :build()

local resource_validator = validator_builder.new()
    :named("resource")
    :require_resource_prefix("doc/")
    :build()

local auth_rule = rule_builder.new()
    :named("auth")
    :for_kind("auth")
    :cache_with("auth")
    :decorate(function(reason: string, _policy: protocol.TenantPolicy, request: protocol.Request): string
        if request.kind == "auth" then
            return reason .. ":" .. request.actor_id
        end
        return reason
    end)
    :build()

local query_rule = rule_builder.new()
    :named("query")
    :for_kind("query")
    :fallback_to("policy-review")
    :cache_with("query")
    :decorate(function(reason: string, _policy: protocol.TenantPolicy, request: protocol.Request): string
        if request.kind == "query" then
            return reason .. ":" .. request.resource
        end
        return reason
    end)
    :build()

local update_rule = rule_builder.new()
    :named("update")
    :for_kind("update")
    :fallback_to("manual-approval")
    :cache_with("update")
    :decorate(function(reason: string, _policy: protocol.TenantPolicy, request: protocol.Request): string
        if request.kind == "update" then
            return reason .. ":" .. request.resource
        end
        return reason
    end)
    :build()

local app = runtime.new()
    :use_validator(source_validator)
    :use_validator(auth_validator)
    :use_validator(resource_validator)
    :register_evaluator("auth", auth_rule)
    :register_evaluator("query", query_rule)
    :register_evaluator("update", update_rule)

app:on_step(function(step: protocol.PolicyStep, state: protocol.StoreState)
    last_runtime_id = state.id
    if step.kind == "decision" then
        observed_decisions[step.tenant_id .. ":" .. step.request_kind] = step.note
    elseif step.kind == "defer" then
        table.insert(observed_defers, step.note)
        local retry_seconds: integer = step.retry_at:unix()
        local queue: string = step.queue
    else
        table.insert(observed_audits, step.note)
        local at_seconds: integer = step.at:unix()
    end
end)

local store = app:new_store("policy-1", now)

store:save_policy({
    tenant_id = "tenant-a",
    scopes = {["policy.read"] = true},
    allowed_resources = {["doc/alpha"] = true, ["doc/beta"] = true},
    fallback_queue = "policy-review",
    tags = {source = "api"},
    last_checked = nil,
})

store:save_policy({
    tenant_id = "tenant-b",
    scopes = {["policy.read"] = true},
    allowed_resources = {},
    fallback_queue = "manual-approval",
    tags = {source = "worker"},
    last_checked = nil,
})

local auth_one: protocol.AuthRequest = {
    kind = "auth",
    tenant_id = "tenant-a",
    actor_id = "actor-1",
    scope = "policy.read",
    meta = protocol.meta("trace-1", {source = "api"}),
}

local query_one: protocol.QueryRequest = {
    kind = "query",
    tenant_id = "tenant-a",
    actor_id = "actor-1",
    resource = "doc/alpha",
    meta = protocol.meta("trace-2", {source = "api", priority = "high"}),
}

local update_two: protocol.UpdateRequest = {
    kind = "update",
    tenant_id = "tenant-b",
    actor_id = "actor-2",
    resource = "doc/review",
    change_set = {status = "submitted"},
    meta = protocol.meta("trace-3", {source = "worker"}),
}

local tick: protocol.TickRequest = {
    kind = "tick",
    at = now,
}

local requests: {protocol.Request} = {
    auth_one,
    query_one,
    update_two,
    tick,
}

local summary_result = app:run(store, requests, now)
if not summary_result.ok then
    local message: string = summary_result.error.message
    local retryable: boolean = summary_result.error.retryable
else
    local summary = summary_result.value
    local runtime_id: string = summary.id
    local total_processed: number = summary.total_processed
    local allowed_count: number = summary.allowed_count
    local denied_count: number = summary.denied_count
    local deferred_count: number = summary.deferred_count
    local elapsed_seconds: time.Duration = summary.elapsed_seconds
    local last_kind: string? = summary.last_kind
end

local summary_label = result.map(summary_result, function(summary: protocol.RunSummary): string
    return summary.id .. ":" .. tostring(summary.allowed_count + summary.deferred_count)
end)

if summary_label.ok then
    local label: string = summary_label.value
end

local checked_result = result.and_then(summary_result, function(summary: protocol.RunSummary): StringResult
    if summary.deferred_count == 0 then
        return {
            ok = false,
            error = {
                code = "invalid",
                message = "expected deferred decision",
                retryable = false,
            },
        }
    end
    return {ok = true, value = summary.id}
end)

if checked_result.ok then
    local checked_id: string = checked_result.value
end

local policy_one = store:lookup_policy("tenant-a")
if policy_one then
    local tenant_id: string = policy_one.tenant_id
    local tags = policy_one.tags
    if tags then
        local origin = tags["source"]
        if origin then
            local source: string = origin
        end
    end
    local last_checked = policy_one.last_checked
    if last_checked then
        local checked_at: integer = last_checked:unix()
    end
end

local cached_auth = store:lookup_decision("auth:tenant-a:actor-1")
if cached_auth then
    if cached_auth.kind == "allow" then
        local allow_decision: protocol.AllowDecision = cached_auth
        local reason: string = allow_decision.reason
        local cache_key = allow_decision.cache_key
        if cache_key then
            local key: string = cache_key
        end
        local expires_at = allow_decision.expires_at
        if expires_at then
            local expires_seconds: integer = expires_at:unix()
        end
    end
end

local cached_query = store:lookup_decision("query:tenant-a:doc/alpha")
if cached_query then
    if cached_query.kind == "allow" then
        local allow_decision: protocol.AllowDecision = cached_query
        local reason: string = allow_decision.reason
    end
end

local cached_defer = store:lookup_decision("tenant-b:defer:update")
if cached_defer then
    if cached_defer.kind == "defer" then
        local defer_decision: protocol.DeferDecision = cached_defer
        local queue: string = defer_decision.queue
        local retry_seconds: integer = defer_decision.retry_at:unix()
    end
end

local allowed_counter = store.state.counters["allow"]
if allowed_counter then
    local allowed_value: integer = allowed_counter
end

local deferred_counter = store.state.counters["defer"]
if deferred_counter then
    local deferred_value: integer = deferred_counter
end

local saw_source = store.state.flags["saw_source"]
if saw_source then
    local flag: boolean = saw_source
end

local last_eval_at = store.state.last_eval_at
if last_eval_at then
    local last_eval_seconds: integer = last_eval_at:unix()
end

local source_tag = helpers.tag_value(query_one, "source")
if source_tag then
    local source: string = source_tag
end

if last_runtime_id then
    local runtime_id: string = last_runtime_id
end
