local time = require("time")
local protocol = require("protocol")
local helpers = require("helpers")
local saga_store = require("saga_store")

type Orchestrator = {
    validators: {protocol.ActionValidator},
    handlers: {[string]: protocol.ActionHandler},
    hooks: {protocol.StepHook},
    use_validator: (self: Orchestrator, validator: protocol.ActionValidator) -> Orchestrator,
    register_handler: (self: Orchestrator, kind: string, handler: protocol.ActionHandler) -> Orchestrator,
    on_step: (self: Orchestrator, hook: protocol.StepHook) -> Orchestrator,
    new_store: (self: Orchestrator, id: string, now: time.Time) -> saga_store.SagaStore,
    emit: (self: Orchestrator, store: saga_store.SagaStore, step: protocol.SagaStep, at: time.Time) -> (),
    execute: (self: Orchestrator, store: saga_store.SagaStore, action: protocol.Action, at: time.Time) -> protocol.ExecuteResult,
    run: (self: Orchestrator, store: saga_store.SagaStore, actions: {protocol.Action}, now: time.Time) -> protocol.RunResult,
}

type Runtime = Orchestrator

local Runtime = {}
Runtime.__index = Runtime

local M = {}
M.Orchestrator = Orchestrator

function M.new(): Orchestrator
    local self: Runtime = {
        validators = {},
        handlers = {},
        hooks = {},
        use_validator = Runtime.use_validator,
        register_handler = Runtime.register_handler,
        on_step = Runtime.on_step,
        new_store = Runtime.new_store,
        emit = Runtime.emit,
        execute = Runtime.execute,
        run = Runtime.run,
    }
    setmetatable(self, Runtime)
    return self
end

function Runtime:use_validator(validator: protocol.ActionValidator): Runtime
    table.insert(self.validators, validator)
    return self
end

function Runtime:register_handler(kind: string, handler: protocol.ActionHandler): Runtime
    self.handlers[kind] = handler
    return self
end

function Runtime:on_step(hook: protocol.StepHook): Runtime
    table.insert(self.hooks, hook)
    return self
end

function Runtime:new_store(id: string, now: time.Time): saga_store.SagaStore
    return saga_store.new(id, now)
end

function Runtime:emit(store: saga_store.SagaStore, step: protocol.SagaStep, at: time.Time)
    store:push_step(step, at)
    for _, hook in ipairs(self.hooks) do
        hook(step, store.state)
    end
end

function Runtime:execute(
    store: saga_store.SagaStore,
    action: protocol.Action,
    at: time.Time
): protocol.ExecuteResult
    if action.kind == "tick" then
        local audit_step: protocol.SagaStep = {
            kind = "audit",
            note = "tick",
            at = action.at,
        }
        self:emit(store, audit_step, at)
        return {ok = true, value = nil}
    end

    local current: protocol.Action = action
    for _, validator in ipairs(self.validators) do
        local validation_result: protocol.ValidationResult = validator(store.state, current)
        if not validation_result.ok then
            return {ok = false, error = validation_result.error}
        end
        current = validation_result.value
    end

    local handler = self.handlers[current.kind]
    if not handler then
        return {
            ok = false,
            error = {
                code = "not_found",
                message = "missing handler: " .. current.kind,
                retryable = false,
            },
        }
    end

    local handler_value: protocol.ActionHandler = handler
    local handler_result: protocol.HandlerResult = handler_value(store.state, current, at)
    if not handler_result.ok then
        return {ok = false, error = handler_result.error}
    end

    local note = handler_result.value or helpers.action_label(current)
    local action_step: protocol.SagaStep = {
        kind = "action",
        name = current.kind,
        note = note,
        order_id = current.order_id,
    }
    self:emit(store, action_step, at)

    if action.kind == "cancel" then
        local saga = store:lookup_saga(action.order_id)
        if saga then
            for i = #saga.compensations, 1, -1 do
                local comp = saga.compensations[i]
                local comp_note: string
                local comp_name: string
                if comp.kind == "release" then
                    comp_name = "release"
                    comp_note = "release:" .. comp.reservation_token
                else
                    comp_name = "refund"
                    comp_note = "refund:" .. comp.payment_id
                end
                local comp_step: protocol.SagaStep = {
                    kind = "compensation",
                    name = comp_name,
                    note = comp_note,
                    order_id = saga.order_id,
                }
                self:emit(store, comp_step, at)
            end
        end
    end

    return {ok = true, value = current.kind}
end

function Runtime:run(
    store: saga_store.SagaStore,
    actions: {protocol.Action},
    now: time.Time
): protocol.RunResult
    local last_status: string? = nil

    for _, action in ipairs(actions) do
        local execute_result: protocol.ExecuteResult = self:execute(store, action, now)
        if not execute_result.ok then
            return {ok = false, error = execute_result.error}
        end
        if execute_result.value then
            last_status = execute_result.value
        end
    end

    return {ok = true, value = store:summarize(now, last_status)}
end

return M
