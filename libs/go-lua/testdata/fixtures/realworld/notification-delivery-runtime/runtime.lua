local time = require("time")
local result = require("result")
local protocol = require("protocol")
local helpers = require("helpers")
local delivery_store = require("delivery_store")

type DeliveryRuntime = {
    transports: {[string]: protocol.TransportHandler},
    hooks: {protocol.StepHook},
    register_transport: (self: DeliveryRuntime, kind: string, handler: protocol.TransportHandler) -> DeliveryRuntime,
    on_step: (self: DeliveryRuntime, hook: protocol.StepHook) -> DeliveryRuntime,
    new_store: (self: DeliveryRuntime, id: string, now: time.Time) -> delivery_store.DeliveryStore,
    emit: (self: DeliveryRuntime, store: delivery_store.DeliveryStore, step: protocol.DeliveryStep, at: time.Time) -> (),
    deliver: (self: DeliveryRuntime, store: delivery_store.DeliveryStore, request: protocol.Request, at: time.Time) -> protocol.DeliverResult,
    run: (self: DeliveryRuntime, store: delivery_store.DeliveryStore, requests: {protocol.Request}, now: time.Time) -> protocol.RunResult,
}

type Runtime = DeliveryRuntime

local Runtime = {}
Runtime.__index = Runtime

local M = {}
M.DeliveryRuntime = DeliveryRuntime

function M.new(): DeliveryRuntime
    local self: Runtime = {
        transports = {},
        hooks = {},
        register_transport = Runtime.register_transport,
        on_step = Runtime.on_step,
        new_store = Runtime.new_store,
        emit = Runtime.emit,
        deliver = Runtime.deliver,
        run = Runtime.run,
    }
    setmetatable(self, Runtime)
    return self
end

function Runtime:register_transport(kind: string, handler: protocol.TransportHandler): Runtime
    self.transports[kind] = handler
    return self
end

function Runtime:on_step(hook: protocol.StepHook): Runtime
    table.insert(self.hooks, hook)
    return self
end

function Runtime:new_store(id: string, now: time.Time): delivery_store.DeliveryStore
    return delivery_store.new(id, now)
end

function Runtime:emit(store: delivery_store.DeliveryStore, step: protocol.DeliveryStep, at: time.Time)
    store:push_step(step, at)
    for _, hook in ipairs(self.hooks) do
        hook(step, store.state)
    end
end

function Runtime:deliver(
    store: delivery_store.DeliveryStore,
    request: protocol.Request,
    at: time.Time
): protocol.DeliverResult
    if request.kind == "tick" then
        local audit_step: protocol.AuditStep = {kind = "audit", note = "tick", at = request.at}
        self:emit(store, audit_step, at)
        return {ok = true, value = nil}
    end

    local handler = self.transports[request.kind]
    if not handler then
        return {
            ok = false,
            error = {
                code = "not_found",
                message = "missing transport: " .. request.kind,
                retryable = false,
            },
        }
    end

    local receipt_result = handler(store.state, request, at)
    if not receipt_result.ok then
        return {ok = false, error = receipt_result.error}
    end

    local receipt = receipt_result.value
    store:record_receipt(request, receipt)

    if receipt.local_status == "retrying" then
        local retry_at = receipt.retry_after or at
        local retry_step: protocol.RetryStep = {
            kind = "retry",
            channel = receipt.channel,
            message_id = receipt.message_id,
            note = helpers.receipt_note(receipt),
            retry_at = retry_at,
        }
        self:emit(store, retry_step, at)
        return {ok = true, value = "retrying"}
    end

    local delivery_step: protocol.DeliveryEventStep = {
        kind = "delivery",
        channel = receipt.channel,
        message_id = receipt.message_id,
        note = helpers.receipt_note(receipt),
        provider_id = receipt.provider_id,
    }
    self:emit(store, delivery_step, at)
    return {ok = true, value = receipt.local_status}
end

function Runtime:run(
    store: delivery_store.DeliveryStore,
    requests: {protocol.Request},
    now: time.Time
): protocol.RunResult
    local last_status: string? = nil

    for _, request in ipairs(requests) do
        local deliver_result: protocol.DeliverResult = self:deliver(store, request, now)
        if not deliver_result.ok then
            return {ok = false, error = deliver_result.error}
        end
        if deliver_result.value then
            last_status = deliver_result.value
        end
    end

    return {ok = true, value = store:summarize(now, last_status)}
end

return M
