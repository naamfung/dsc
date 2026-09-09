local time = require("time")
local protocol = require("protocol")
local subscriber_builder = require("subscriber_builder")
local projector_builder = require("projector_builder")
local bus = require("bus")

local now = time.now()

local subscriber = subscriber_builder.new()
    :named("source")
    :prefix_with("evt")
    :build()

local projector = projector_builder.new()
    :track_queue("priority")
    :count_failures_as("failed")
    :capture_source("source")
    :build()

local app = bus.new()
    :register_projector("tasks", projector)
    :register_subscriber("tasks", subscriber)

local queued_one: protocol.TaskQueuedEvent = {
    kind = "queued",
    id = "job-1",
    queue = "priority",
    payload = {task = "search"},
    meta = protocol.meta("trace-1", nil),
}

local tick: protocol.TickEvent = {
    kind = "tick",
    at = now,
}

local store = app:new_store("bus-1", now)
local replay_result = app:replay(store, "tasks", {queued_one, tick}, now)

if replay_result.ok then
    local last_status: string = replay_result.value.last_status -- expect-error
end

local projection = store:lookup_projection("job-1")
local output: string = projection.output -- expect-error
local source: string = projection.source -- expect-error

local missing: protocol.TaskProjection = store.state.projections["missing"] -- expect-error

local elapsed = now:sub(store.state.last_event_at) -- expect-error
local trace_source: string = queued_one.meta.tags["source"] -- expect-error
