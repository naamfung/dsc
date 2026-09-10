local time = require("time")
local protocol = require("protocol")
local helpers = require("helpers")
local policy_store = require("policy_store")

type PolicyRuntime = {
    validators: {protocol.RequestValidator},
    evaluators: {[string]: protocol.PolicyEvaluator},
    hooks: {protocol.StepHook},
    use_validator: (self: PolicyRuntime, validator: protocol.RequestValidator) -> PolicyRuntime,
    register_evaluator: (self: PolicyRuntime, request_kind: string, evaluator: protocol.PolicyEvaluator) -> PolicyRuntime,
    on_step: (self: PolicyRuntime, hook: protocol.StepHook) -> PolicyRuntime,
    new_store: (self: PolicyRuntime, id: string, now: time.Time) -> policy_store.PolicyStore,
    emit: (self: PolicyRuntime, store: policy_store.PolicyStore, step: protocol.PolicyStep, at: time.Time) -> (),
    evaluate: (self: PolicyRuntime, store: policy_store.PolicyStore, request: protocol.Request, at: time.Time) -> protocol.EvaluateResult,
    run: (self: PolicyRuntime, store: policy_store.PolicyStore, requests: {protocol.Request}, now: time.Time) -> protocol.RunResult,
}

type Runtime = PolicyRuntime

local Runtime = {}
Runtime.__index = Runtime

local M = {}
M.PolicyRuntime = PolicyRuntime

function M.new(): PolicyRuntime
    local self: Runtime = {
        validators = {},
        evaluators = {},
        hooks = {},
        use_validator = Runtime.use_validator,
        register_evaluator = Runtime.register_evaluator,
        on_step = Runtime.on_step,
        new_store = Runtime.new_store,
        emit = Runtime.emit,
        evaluate = Runtime.evaluate,
        run = Runtime.run,
    }
    setmetatable(self, Runtime)
    return self
end

function Runtime:use_validator(validator: protocol.RequestValidator): Runtime
    table.insert(self.validators, validator)
    return self
end

function Runtime:register_evaluator(request_kind: string, evaluator: protocol.PolicyEvaluator): Runtime
    self.evaluators[request_kind] = evaluator
    return self
end

function Runtime:on_step(hook: protocol.StepHook): Runtime
    table.insert(self.hooks, hook)
    return self
end

function Runtime:new_store(id: string, now: time.Time): policy_store.PolicyStore
    return policy_store.new(id, now)
end

function Runtime:emit(store: policy_store.PolicyStore, step: protocol.PolicyStep, at: time.Time)
    store:push_step(step, at)
    for _, hook in ipairs(self.hooks) do
        hook(step, store.state)
    end
end

function Runtime:evaluate(
    store: policy_store.PolicyStore,
    request: protocol.Request,
    at: time.Time
): protocol.EvaluateResult
    if request.kind == "tick" then
        local audit_step: protocol.AuditStep = {kind = "audit", note = "tick", at = request.at}
        self:emit(store, audit_step, at)
        return {ok = true, value = nil}
    end

    for _, validator in ipairs(self.validators) do
        local validation: protocol.ValidationResult = validator(store.state, request)
        if not validation.ok then
            return {ok = false, error = validation.error}
        end
    end

    local evaluator = self.evaluators[request.kind]
    if not evaluator then
        return {
            ok = false,
            error = {
                code = "not_found",
                message = "missing evaluator: " .. request.kind,
                retryable = false,
            },
        }
    end

    local evaluation = evaluator(store.state, request, at)
    if not evaluation.ok then
        return {ok = false, error = evaluation.error}
    end

    local decision = evaluation.value
    if not decision then
        return {ok = true, value = nil}
    end

    store:cache_decision(request, decision, at)

    if decision.kind == "defer" then
        local defer_decision: protocol.DeferDecision = decision
        local defer_step: protocol.DeferStep = {
            kind = "defer",
            tenant_id = request.tenant_id,
            queue = defer_decision.queue,
            note = helpers.decision_note(defer_decision),
            retry_at = defer_decision.retry_at,
        }
        self:emit(store, defer_step, at)
        return {ok = true, value = defer_decision}
    end

    local decision_step: protocol.DecisionStep = {
        kind = "decision",
        request_kind = request.kind,
        tenant_id = request.tenant_id,
        note = helpers.decision_note(decision),
    }
    self:emit(store, decision_step, at)
    return {ok = true, value = decision}
end

function Runtime:run(
    store: policy_store.PolicyStore,
    requests: {protocol.Request},
    now: time.Time
): protocol.RunResult
    local last_kind: string? = nil

    for _, request in ipairs(requests) do
        local evaluation: protocol.EvaluateResult = self:evaluate(store, request, now)
        if not evaluation.ok then
            return {ok = false, error = evaluation.error}
        end

        local decision = evaluation.value
        if decision then
            last_kind = decision.kind
        end
    end

    return {ok = true, value = store:summarize(now, last_kind)}
end

return M
