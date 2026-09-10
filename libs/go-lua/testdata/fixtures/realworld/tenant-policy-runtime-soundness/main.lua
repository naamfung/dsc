local time = require("time")
local protocol = require("protocol")
local validator_builder = require("validator_builder")
local rule_builder = require("rule_builder")
local runtime = require("runtime")

local now = time.now()

local source_validator = validator_builder.new()
    :named("source")
    :require_tag("source")
    :build()

local auth_rule = rule_builder.new()
    :named("auth")
    :for_kind("auth")
    :cache_with("auth")
    :build()

local app = runtime.new()
    :use_validator(source_validator)
    :register_evaluator("auth", auth_rule)

local store = app:new_store("policy-soundness", now)

store:save_policy({
    tenant_id = "tenant-a",
    scopes = {["policy.read"] = true},
    allowed_resources = {},
    fallback_queue = nil,
    tags = nil,
    last_checked = nil,
})

local auth_one: protocol.AuthRequest = {
    kind = "auth",
    tenant_id = "tenant-a",
    actor_id = "actor-1",
    scope = "policy.read",
    meta = protocol.meta("trace-1", {source = "api"}),
}

local tick: protocol.TickRequest = {
    kind = "tick",
    at = now,
}

local run_result = app:run(store, {auth_one, tick}, now)

if run_result.ok then
    local last_kind: string = run_result.value.last_kind -- expect-error
end

local missing_policy: protocol.TenantPolicy = store:lookup_policy("missing") -- expect-error
local missing_decision: protocol.Decision = store.state.cached_decisions["missing"] -- expect-error
local evaluator: protocol.PolicyEvaluator = app.evaluators["auth"] -- expect-error

local elapsed = now:sub(store.state.last_eval_at) -- expect-error
local source: string = protocol.meta("trace-2", nil).tags["source"] -- expect-error

local cached = store:lookup_decision("auth:tenant-a:actor-1")
if cached then
    local queue: string = cached.queue -- expect-error
end

local missing_status: string = store:lookup_policy("tenant-a").tags["source"] -- expect-error
