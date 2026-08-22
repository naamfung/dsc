local time = require("time")
local result = require("result")
local protocol = require("protocol")
local helpers = require("helpers")
local validator_builder = require("validator_builder")
local action_builder = require("action_builder")
local orchestrator = require("orchestrator")

type StringResult = {ok: true, value: string} | {ok: false, error: result.AppError}

local now = time.now()

local observed_actions: {[string]: string} = {}
local observed_compensations: {string} = {}
local observed_audits: {string} = {}
local last_runtime_id: string? = nil

local source_validator = validator_builder.new()
    :named("source")
    :require_tag("source")
    :remember_flag("validated_source")
    :build()

local begin_handler = action_builder.new()
    :for_kind("begin")
    :prefix_with("begin")
    :count_as("begun")
    :capture_source("source")
    :decorate(function(note: string, saga: protocol.SagaAggregate, action: protocol.Action): string
        return note .. ":" .. saga.customer_id .. ":" .. helpers.action_label(action)
    end)
    :build()

local reserve_handler = action_builder.new()
    :for_kind("reserve")
    :prefix_with("reserve")
    :count_as("reserved")
    :capture_source("source")
    :decorate(function(note: string, saga: protocol.SagaAggregate, _action: protocol.Action): string
        local token = saga.reservation_token or "missing"
        return note .. ":" .. token
    end)
    :build()

local charge_handler = action_builder.new()
    :for_kind("charge")
    :prefix_with("charge")
    :count_as("charged")
    :capture_source("source")
    :decorate(function(note: string, saga: protocol.SagaAggregate, _action: protocol.Action): string
        local payment = saga.payment_id or "missing"
        return note .. ":" .. payment
    end)
    :build()

local commit_handler = action_builder.new()
    :for_kind("commit")
    :prefix_with("commit")
    :count_as("committed")
    :capture_source("source")
    :decorate(function(note: string, saga: protocol.SagaAggregate, _action: protocol.Action): string
        local source = saga.source or "unknown"
        return note .. ":" .. source
    end)
    :build()

local cancel_handler = action_builder.new()
    :for_kind("cancel")
    :prefix_with("cancel")
    :count_as("cancelled")
    :capture_source("source")
    :decorate(function(note: string, saga: protocol.SagaAggregate, action: protocol.Action): string
        local err = saga.last_error or "none"
        return note .. ":" .. err .. ":" .. helpers.action_label(action)
    end)
    :build()

local app = orchestrator.new()
    :use_validator(source_validator)
    :register_handler("begin", begin_handler)
    :register_handler("reserve", reserve_handler)
    :register_handler("charge", charge_handler)
    :register_handler("commit", commit_handler)
    :register_handler("cancel", cancel_handler)

app:on_step(function(step: protocol.SagaStep, state: protocol.StoreState)
    last_runtime_id = state.id

    if step.kind == "action" then
        observed_actions[step.name .. ":" .. tostring(#observed_audits + 1)] = step.note
        if step.order_id then
            local order_id: string = step.order_id
        end
    elseif step.kind == "compensation" then
        table.insert(observed_compensations, step.note)
        if step.order_id then
            local order_id: string = step.order_id
        end
    else
        table.insert(observed_audits, step.note)
        local at_seconds: integer = step.at:unix()
    end
end)

local begin_one: protocol.BeginAction = {
    kind = "begin",
    order_id = "ord-1",
    customer_id = "alice",
    meta = protocol.meta("trace-1", {source = "api"}),
}

local reserve_one: protocol.ReserveAction = {
    kind = "reserve",
    order_id = "ord-1",
    sku = "sku-1",
    qty = 2,
    meta = protocol.meta("trace-2", {source = "worker"}),
}

local charge_one: protocol.ChargeAction = {
    kind = "charge",
    order_id = "ord-1",
    cents = 4200,
    meta = protocol.meta("trace-3", {source = "worker"}),
}

local commit_one: protocol.CommitAction = {
    kind = "commit",
    order_id = "ord-1",
    meta = protocol.meta("trace-4", {source = "worker"}),
}

local begin_two: protocol.BeginAction = {
    kind = "begin",
    order_id = "ord-2",
    customer_id = "bob",
    meta = protocol.meta("trace-5", {source = "api"}),
}

local reserve_two: protocol.ReserveAction = {
    kind = "reserve",
    order_id = "ord-2",
    sku = "sku-2",
    qty = 1,
    meta = protocol.meta("trace-6", {source = "worker"}),
}

local charge_two: protocol.ChargeAction = {
    kind = "charge",
    order_id = "ord-2",
    cents = 9900,
    meta = protocol.meta("trace-7", {source = "worker"}),
}

local cancel_two: protocol.CancelAction = {
    kind = "cancel",
    order_id = "ord-2",
    reason = "payment_declined",
    meta = protocol.meta("trace-8", {source = "worker"}),
}

local tick: protocol.TickAction = {
    kind = "tick",
    at = now,
}

local actions: {protocol.Action} = {
    begin_one,
    reserve_one,
    charge_one,
    commit_one,
    begin_two,
    reserve_two,
    charge_two,
    cancel_two,
    tick,
}

local store = app:new_store("saga-1", now)
local summary_result = app:run(store, actions, now)
if not summary_result.ok then
    local message: string = summary_result.error.message
    local retryable: boolean = summary_result.error.retryable
else
    local summary = summary_result.value
    local runtime_id: string = summary.id
    local total_steps: number = summary.total_steps
    local saga_count: number = summary.saga_count
    local committed_count: number = summary.committed_count
    local rolled_back_count: number = summary.rolled_back_count
    local elapsed_seconds: number = summary.elapsed_seconds
    local last_status: string? = summary.last_status
end

local summary_label = result.map(summary_result, function(summary: protocol.RunSummary): string
    return summary.id .. ":" .. tostring(summary.saga_count)
end)

if summary_label.ok then
    local label: string = summary_label.value
end

local summary_id = result.and_then(summary_result, function(summary: protocol.RunSummary): StringResult
    if summary.rolled_back_count == 0 then
        return {
            ok = false,
            error = {
                code = "invalid",
                message = "expected rollback",
                retryable = false,
            },
        }
    end
    return {
        ok = true,
        value = summary.id,
    }
end)

if summary_id.ok then
    local stable_id: string = summary_id.value
end

local saga_one = store:lookup_saga("ord-1")
if saga_one then
    local status: string = saga_one.status
    local version: integer = saga_one.version
    local reservation = saga_one.reservation_token
    if reservation then
        local stable_reservation: string = reservation
    end
    local payment = saga_one.payment_id
    if payment then
        local stable_payment: string = payment
    end
    local source = saga_one.source
    if source then
        local stable_source: string = source
    end
    local updated = saga_one.updated_at or now
    local seconds: number = now:sub(updated):seconds()
end

local saga_two = store:lookup_saga("ord-2")
if saga_two then
    local failed_status: string = saga_two.status
    local error_msg = saga_two.last_error
    if error_msg then
        local stable_error: string = error_msg
    end
    for _, comp in ipairs(saga_two.compensations) do
        if comp.kind == "release" then
            local token: string = comp.reservation_token
        else
            local payment_id: string = comp.payment_id
        end
    end
end

local view_one = store:lookup_view("ord-1")
if view_one then
    local committed = view_one.committed_at
    if committed then
        local unix_seconds: integer = committed:unix()
    end
end

local view_two = store:lookup_view("ord-2")
if view_two then
    local rolled_back = view_two.rolled_back_at
    if rolled_back then
        local unix_seconds: integer = rolled_back:unix()
    end
    local source = view_two.source
    if source then
        local stable_source: string = source
    end
end

for key, note in pairs(observed_actions) do
    local stable_key: string = key
    local stable_note: string = note
end

for _, note in ipairs(observed_compensations) do
    local stable_note: string = note
end

for _, note in ipairs(observed_audits) do
    local audit_note: string = note
end

if last_runtime_id ~= nil then
    local stable_runtime_id: string = last_runtime_id
end

local source = helpers.source_tag(begin_one)
if source then
    local stable_source: string = source
end

local seen_at = store.state.last_action_at or store.state.started_at
local elapsed = now:sub(seen_at)
local seconds: number = elapsed:seconds()
