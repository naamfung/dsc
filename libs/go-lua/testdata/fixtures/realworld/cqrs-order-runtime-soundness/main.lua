local time = require("time")
local protocol = require("protocol")
local validator_builder = require("validator_builder")
local handler_builder = require("handler_builder")
local runtime = require("runtime")

local now = time.now()

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
    :build()

local app = runtime.new()
    :use_validator(source_validator)
    :register_handler("create", create_handler)

local create_one: protocol.CreateOrderCommand = {
    kind = "create",
    id = "ord-1",
    customer = "alice",
    meta = protocol.meta("trace-1", {source = "api"}),
}

local tick: protocol.TickCommand = {
    kind = "tick",
    at = now,
}

local store = app:new_store("cqrs-1", now)
local replay_result = app:replay(store, {create_one, tick}, now)

if replay_result.ok then
    local last_status: string = replay_result.value.last_status -- expect-error
end

local order = store:lookup_order("ord-1")
local item_id: string = order.item_id -- expect-error
local order_source: string = order.source -- expect-error

local view = store:lookup_view("ord-1")
local completed_at = now:sub(view.completed_at) -- expect-error

local missing_view: protocol.OrderView = store.state.views["missing"] -- expect-error
local trace_source: string = create_one.meta.tags["source"] -- expect-error
