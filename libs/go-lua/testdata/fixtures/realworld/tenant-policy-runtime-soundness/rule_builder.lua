local protocol = require("protocol")
local helpers = require("helpers")

type RuleDecorator = (string, protocol.TenantPolicy, protocol.Request) -> string

type RuleBuilder = {
    name: string,
    request_kind: "auth" | "query" | "update",
    required_scope: string?,
    fallback_queue: string?,
    cache_prefix: string?,
    decorator: RuleDecorator?,
    named: (self: RuleBuilder, name: string) -> RuleBuilder,
    for_kind: (self: RuleBuilder, request_kind: "auth" | "query" | "update") -> RuleBuilder,
    require_scope: (self: RuleBuilder, scope: string) -> RuleBuilder,
    fallback_to: (self: RuleBuilder, queue: string) -> RuleBuilder,
    cache_with: (self: RuleBuilder, prefix: string) -> RuleBuilder,
    decorate: (self: RuleBuilder, decorator: RuleDecorator) -> RuleBuilder,
    build: (self: RuleBuilder) -> protocol.PolicyEvaluator,
}

type Builder = RuleBuilder

local Builder = {}
Builder.__index = Builder

local M = {}

function M.new(): RuleBuilder
    local self: Builder = {
        name = "rule",
        request_kind = "auth",
        required_scope = nil,
        fallback_queue = nil,
        cache_prefix = nil,
        decorator = nil,
        named = Builder.named,
        for_kind = Builder.for_kind,
        require_scope = Builder.require_scope,
        fallback_to = Builder.fallback_to,
        cache_with = Builder.cache_with,
        decorate = Builder.decorate,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:named(name: string): Builder
    self.name = name
    return self
end

function Builder:for_kind(request_kind: "auth" | "query" | "update"): Builder
    self.request_kind = request_kind
    return self
end

function Builder:require_scope(scope: string): Builder
    self.required_scope = scope
    return self
end

function Builder:fallback_to(queue: string): Builder
    self.fallback_queue = queue
    return self
end

function Builder:cache_with(prefix: string): Builder
    self.cache_prefix = prefix
    return self
end

function Builder:decorate(decorator: RuleDecorator): Builder
    self.decorator = decorator
    return self
end

function Builder:build(): protocol.PolicyEvaluator
    local name = self.name
    local request_kind = self.request_kind
    local required_scope = self.required_scope
    local fallback_queue = self.fallback_queue
    local cache_prefix = self.cache_prefix
    local decorator = self.decorator

    return function(state: protocol.StoreState, request: protocol.Request, at: time.Time): protocol.EvaluateResult
        if request.kind == "tick" then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = name .. " cannot evaluate ticks",
                    retryable = false,
                },
            }
        end

        if request.kind ~= request_kind then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = name .. " wrong request kind: " .. helpers.request_label(request),
                    retryable = false,
                },
            }
        end

        local policy = state.policies[request.tenant_id]
        if not policy then
            return {
                ok = false,
                error = {
                    code = "not_found",
                    message = name .. " missing tenant policy",
                    retryable = false,
                },
            }
        end

        local reason = name .. ":" .. request.tenant_id
        if decorator then
            reason = decorator(reason, policy, request)
        end

        if request.kind == "auth" then
            local scope_key = required_scope or request.scope
            if policy.scopes[scope_key] then
                local cache_key: string? = nil
                if cache_prefix then
                    cache_key = cache_prefix .. ":" .. request.tenant_id .. ":" .. request.actor_id
                end
                local allow: protocol.AllowDecision = {
                    kind = "allow",
                    reason = reason,
                    cache_key = cache_key,
                    expires_at = at,
                }
                return {ok = true, value = allow}
            end

            local deny: protocol.DenyDecision = {
                kind = "deny",
                reason = reason,
                retryable = false,
            }
            return {ok = true, value = deny}
        end

        local resource: string
        if request.kind == "query" then
            resource = request.resource
        else
            resource = request.resource
        end

        if policy.allowed_resources[resource] then
            local cache_key: string? = nil
            if cache_prefix then
                cache_key = cache_prefix .. ":" .. request.tenant_id .. ":" .. resource
            end
            local allow: protocol.AllowDecision = {
                kind = "allow",
                reason = reason,
                cache_key = cache_key,
                expires_at = at,
            }
            return {ok = true, value = allow}
        end

        local queue = fallback_queue or policy.fallback_queue
        if queue then
            local defer: protocol.DeferDecision = {
                kind = "defer",
                queue = queue,
                retry_at = at,
            }
            return {ok = true, value = defer}
        end

        local deny: protocol.DenyDecision = {
            kind = "deny",
            reason = reason,
            retryable = false,
        }
        return {ok = true, value = deny}
    end
end

return M
