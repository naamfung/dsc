local time = require("time")
local result = require("result")
local protocol = require("protocol")
local helpers = require("helpers")
local validator_builder = require("validator_builder")
local handler_builder = require("handler_builder")
local runtime = require("runtime")

type StringResult = {ok: true, value: string} | {ok: false, error: result.AppError}

local now = time.now()

local observed_notes: {[string]: string} = {}
local observed_audits: {string} = {}
local last_runtime_id: string? = nil

local source_validator = validator_builder.new()
    :named("source")
    :require_tag("source")
    :remember_flag("validated_source")
    :build()

local create_handler = handler_builder.new()
    :for_kind("create")
    :prefix_with("order")
    :count_as("created")
    :capture_source("source")
    :decorate(function(note: string, aggregate: protocol.OrderAggregate, command: protocol.Command): string
        return note .. ":" .. aggregate.customer .. ":" .. helpers.command_label(command)
    end)
    :build()

local reserve_handler = handler_builder.new()
    :for_kind("reserve")
    :prefix_with("reserve")
    :count_as("reserved")
    :capture_source("source")
    :decorate(function(note: string, aggregate: protocol.OrderAggregate, _command: protocol.Command): string
        local item = aggregate.item_id or "missing"
        return note .. ":" .. item
    end)
    :build()

local complete_handler = handler_builder.new()
    :for_kind("complete")
    :prefix_with("complete")
    :count_as("completed")
    :capture_source("source")
    :decorate(function(note: string, aggregate: protocol.OrderAggregate, _command: protocol.Command): string
        local source = aggregate.source or "unknown"
        return note .. ":" .. source
    end)
    :build()

local app = runtime.new()
    :use_validator(source_validator)
    :register_handler("create", create_handler)
    :register_handler("reserve", reserve_handler)
    :register_handler("complete", complete_handler)

app:on_step(function(step: protocol.RunStep, state: protocol.StoreState)
    last_runtime_id = state.id

    if step.kind == "command" then
        observed_notes[step.name .. ":" .. tostring(#observed_audits + 1)] = step.note
        if step.order_id then
            local order_id: string = step.order_id
        end
    else
        table.insert(observed_audits, step.note)
        local at_seconds: integer = step.at:unix()
    end
end)

local create_one: protocol.CreateOrderCommand = {
    kind = "create",
    id = "ord-1",
    customer = "alice",
    meta = protocol.meta("trace-1", {source = "api", lane = "priority"}),
}

local reserve_one: protocol.ReserveItemCommand = {
    kind = "reserve",
    id = "ord-1",
    item_id = "item-7",
    meta = protocol.meta("trace-2", {source = "worker"}),
}

local complete_one: protocol.CompleteOrderCommand = {
    kind = "complete",
    id = "ord-1",
    meta = protocol.meta("trace-3", {source = "worker"}),
}

local create_two: protocol.CreateOrderCommand = {
    kind = "create",
    id = "ord-2",
    customer = "bob",
    meta = protocol.meta("trace-4", {source = "api"}),
}

local reserve_two: protocol.ReserveItemCommand = {
    kind = "reserve",
    id = "ord-2",
    item_id = "item-9",
    meta = protocol.meta("trace-5", {source = "worker"}),
}

local tick: protocol.TickCommand = {
    kind = "tick",
    at = now,
}

local commands: {protocol.Command} = {
    create_one,
    reserve_one,
    complete_one,
    create_two,
    reserve_two,
    tick,
}

local store = app:new_store("cqrs-1", now)
local summary_result = app:replay(store, commands, now)
if not summary_result.ok then
    local message: string = summary_result.error.message
    local retryable: boolean = summary_result.error.retryable
else
    local summary = summary_result.value
    local runtime_id: string = summary.id
    local total_steps: number = summary.total_steps
    local order_count: number = summary.order_count
    local completed_count: number = summary.completed_count
    local elapsed_seconds: number = summary.elapsed_seconds
    local last_status: string? = summary.last_status
end

local summary_label = result.map(summary_result, function(summary: protocol.RunSummary): string
    return summary.id .. ":" .. tostring(summary.order_count)
end)

if summary_label.ok then
    local label: string = summary_label.value
end

local summary_id = result.and_then(summary_result, function(summary: protocol.RunSummary): StringResult
    if summary.completed_count == 0 then
        return {
            ok = false,
            error = {
                code = "invalid",
                message = "expected completed order",
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

local order_one = store:lookup_order("ord-1")
if order_one then
    local status: string = order_one.status
    local version: integer = order_one.version
    local item = order_one.item_id
    if item then
        local stable_item: string = item
    end
    local source = order_one.source
    if source then
        local stable_source: string = source
    end
    local updated = order_one.updated_at or now
    local seconds: number = now:sub(updated):seconds()
end

local view_two = store:lookup_view("ord-2")
if view_two then
    local view_status: string = view_two.status
    local view_version: integer = view_two.version
    local view_item = view_two.item_id
    if view_item then
        local stable_item: string = view_item
    end
end

local missing = store:lookup_view("missing")
if missing == nil then
    local fallback: string = "missing"
end

for key, note in pairs(observed_notes) do
    local stable_key: string = key
    local stable_note: string = note
end

for _, note in ipairs(observed_audits) do
    local audit_note: string = note
end

if last_runtime_id ~= nil then
    local stable_runtime_id: string = last_runtime_id
end

local source = helpers.source_tag(create_one)
if source then
    local stable_source: string = source
end

local seen_at = store.state.last_command_at or store.state.started_at
local elapsed = now:sub(seen_at)
local seconds: number = elapsed:seconds()
