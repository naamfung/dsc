local time = require("time")
local protocol = require("protocol")
local validator_builder = require("validator_builder")
local action_builder = require("action_builder")
local orchestrator = require("orchestrator")

local now = time.now()

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
    :build()

local reserve_handler = action_builder.new()
    :for_kind("reserve")
    :prefix_with("reserve")
    :count_as("reserved")
    :capture_source("source")
    :build()

local cancel_handler = action_builder.new()
    :for_kind("cancel")
    :prefix_with("cancel")
    :count_as("cancelled")
    :capture_source("source")
    :build()

local app = orchestrator.new()
    :use_validator(source_validator)
    :register_handler("begin", begin_handler)
    :register_handler("reserve", reserve_handler)
    :register_handler("cancel", cancel_handler)

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
    qty = 1,
    meta = protocol.meta("trace-2", {source = "worker"}),
}

local cancel_one: protocol.CancelAction = {
    kind = "cancel",
    order_id = "ord-1",
    reason = "manual_stop",
    meta = protocol.meta("trace-3", {source = "worker"}),
}

local tick: protocol.TickAction = {
    kind = "tick",
    at = now,
}

local store = app:new_store("saga-1", now)
local run_result = app:run(store, {begin_one, reserve_one, cancel_one, tick}, now)

if run_result.ok then
    local last_status: string = run_result.value.last_status -- expect-error
end

local saga = store:lookup_saga("ord-1")
local reservation_token: string = saga.reservation_token -- expect-error
local payment_id: string = saga.payment_id -- expect-error

for _, comp in ipairs(saga.compensations) do
    if comp.kind == "release" then
        local token: string = comp.reservation_token
    else
        local payment: string = comp.payment_id
    end
end

local view = store:lookup_view("ord-1")
local committed_seconds = now:sub(view.committed_at) -- expect-error
local trace_source: string = begin_one.meta.tags["source"] -- expect-error
local last_seen = now:sub(store.state.last_action_at) -- expect-error
