local time = require("time")
local protocol = require("protocol")
local helpers = require("helpers")
local order_store = require("order_store")

type Runtime = {
    validators: {protocol.CommandValidator},
    handlers: {[string]: protocol.CommandHandler},
    hooks: {protocol.StepHook},
    use_validator: (self: Runtime, validator: protocol.CommandValidator) -> Runtime,
    register_handler: (self: Runtime, kind: string, handler: protocol.CommandHandler) -> Runtime,
    on_step: (self: Runtime, hook: protocol.StepHook) -> Runtime,
    new_store: (self: Runtime, id: string, now: time.Time) -> order_store.OrderStore,
    emit: (self: Runtime, store: order_store.OrderStore, step: protocol.RunStep, at: time.Time) -> (),
    execute: (self: Runtime, store: order_store.OrderStore, command: protocol.Command, at: time.Time) -> protocol.ExecuteResult,
    replay: (self: Runtime, store: order_store.OrderStore, commands: {protocol.Command}, now: time.Time) -> protocol.RunResult,
}

type AppRuntime = Runtime

local AppRuntime = {}
AppRuntime.__index = AppRuntime

local M = {}
M.Runtime = Runtime

function M.new(): Runtime
    local self: AppRuntime = {
        validators = {},
        handlers = {},
        hooks = {},
        use_validator = AppRuntime.use_validator,
        register_handler = AppRuntime.register_handler,
        on_step = AppRuntime.on_step,
        new_store = AppRuntime.new_store,
        emit = AppRuntime.emit,
        execute = AppRuntime.execute,
        replay = AppRuntime.replay,
    }
    setmetatable(self, AppRuntime)
    return self
end

function AppRuntime:use_validator(validator: protocol.CommandValidator): AppRuntime
    table.insert(self.validators, validator)
    return self
end

function AppRuntime:register_handler(kind: string, handler: protocol.CommandHandler): AppRuntime
    self.handlers[kind] = handler
    return self
end

function AppRuntime:on_step(hook: protocol.StepHook): AppRuntime
    table.insert(self.hooks, hook)
    return self
end

function AppRuntime:new_store(id: string, now: time.Time): order_store.OrderStore
    return order_store.new(id, now)
end

function AppRuntime:emit(store: order_store.OrderStore, step: protocol.RunStep, at: time.Time)
    store:push_step(step, at)
    for _, hook in ipairs(self.hooks) do
        hook(step, store.state)
    end
end

function AppRuntime:execute(
    store: order_store.OrderStore,
    command: protocol.Command,
    at: time.Time
): protocol.ExecuteResult
    if command.kind == "tick" then
        local audit_step: protocol.RunStep = {
            kind = "audit",
            note = "tick",
            at = command.at,
        }
        self:emit(store, audit_step, at)
        return {ok = true, value = nil}
    end

    local current: protocol.Command = command
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
    local handler_value: protocol.CommandHandler = handler
    local handler_result: protocol.HandlerResult = handler_value(store.state, current, at)
    if not handler_result.ok then
        return {ok = false, error = handler_result.error}
    end

    local note = handler_result.value or helpers.command_label(current)
    local command_step: protocol.RunStep = {
        kind = "command",
        name = current.kind,
        note = note,
        order_id = current.id,
    }
    self:emit(store, command_step, at)

    return {ok = true, value = current.kind}
end

function AppRuntime:replay(
    store: order_store.OrderStore,
    commands: {protocol.Command},
    now: time.Time
): protocol.RunResult
    local last_status: string? = nil

    for _, command in ipairs(commands) do
        local execute_result: protocol.ExecuteResult = self:execute(store, command, now)
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
