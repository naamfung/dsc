local time = require("time")
local protocol = require("protocol")
local helpers = require("helpers")

type PolicyStore = {
    state: protocol.StoreState,
    save_policy: (self: PolicyStore, policy: protocol.TenantPolicy) -> (),
    lookup_policy: (self: PolicyStore, tenant_id: string) -> protocol.TenantPolicy?,
    lookup_decision: (self: PolicyStore, cache_key: string) -> protocol.Decision?,
    push_step: (self: PolicyStore, step: protocol.PolicyStep, at: time.Time) -> (),
    cache_decision: (self: PolicyStore, request: protocol.Request, decision: protocol.Decision, at: time.Time) -> (),
    summarize: (self: PolicyStore, now: time.Time, last_kind: string?) -> protocol.RunSummary,
}

type Store = PolicyStore

local Store = {}
Store.__index = Store

local M = {}
M.PolicyStore = PolicyStore

function M.new(id: string, now: time.Time): PolicyStore
    local self: Store = {
        state = {
            id = id,
            started_at = now,
            last_eval_at = nil,
            policies = {},
            cached_decisions = {},
            steps = {},
            counters = {},
            flags = {},
        },
        save_policy = Store.save_policy,
        lookup_policy = Store.lookup_policy,
        lookup_decision = Store.lookup_decision,
        push_step = Store.push_step,
        cache_decision = Store.cache_decision,
        summarize = Store.summarize,
    }
    setmetatable(self, Store)
    return self
end

function Store:save_policy(policy: protocol.TenantPolicy)
    self.state.policies[policy.tenant_id] = policy
end

function Store:lookup_policy(tenant_id: string): protocol.TenantPolicy?
    return self.state.policies[tenant_id]
end

function Store:lookup_decision(cache_key: string): protocol.Decision?
    return self.state.cached_decisions[cache_key]
end

function Store:push_step(step: protocol.PolicyStep, at: time.Time)
    table.insert(self.state.steps, step)
    self.state.last_eval_at = at
end

function Store:cache_decision(request: protocol.Request, decision: protocol.Decision, at: time.Time)
    if request.kind == "tick" then
        return
    end

    local current = self.state.policies[request.tenant_id]
    if current then
        local updated_policy: protocol.TenantPolicy = {
            tenant_id = current.tenant_id,
            scopes = current.scopes,
            allowed_resources = current.allowed_resources,
            fallback_queue = current.fallback_queue,
            tags = current.tags,
            last_checked = at,
        }
        self.state.policies[current.tenant_id] = updated_policy
    end

    if decision.kind == "allow" then
        local key = decision.cache_key
        if key then
            self.state.cached_decisions[key] = decision
        end
    elseif decision.kind == "defer" then
        self.state.cached_decisions[request.tenant_id .. ":defer:" .. request.kind] = decision
    end

    helpers.bump_counter(self.state.counters, decision.kind)
end

function Store:summarize(now: time.Time, last_kind: string?): protocol.RunSummary
    local allowed_count = self.state.counters["allow"] or 0
    local denied_count = self.state.counters["deny"] or 0
    local deferred_count = self.state.counters["defer"] or 0

    return {
        id = self.state.id,
        total_processed = #self.state.steps,
        allowed_count = allowed_count,
        denied_count = denied_count,
        deferred_count = deferred_count,
        elapsed_seconds = now:sub(self.state.started_at),
        last_kind = last_kind,
    }
end

return M
