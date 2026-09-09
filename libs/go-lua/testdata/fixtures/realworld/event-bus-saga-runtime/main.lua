local time = require("time")
local result = require("result")
local protocol = require("protocol")
local helpers = require("helpers")
local subscriber_builder = require("subscriber_builder")
local projector_builder = require("projector_builder")
local bus = require("bus")

type StringResult = {ok: true, value: string} | {ok: false, error: result.AppError}

local now = time.now()

local observed_notes: {[string]: string} = {}
local observed_audits: {string} = {}
local last_bus_id: string? = nil

local source_subscriber = subscriber_builder.new()
    :named("source")
    :prefix_with("evt")
    :require_tag("source")
    :remember_flag("seen_source")
    :decorate(function(note: string, state: protocol.BusState, event: protocol.Event): string
        local suffix = helpers.event_label(event)
        if state.flags["replayed"] then
            suffix = suffix .. ":replayed"
        end
        return note .. ":" .. suffix
    end)
    :build()

local final_subscriber = subscriber_builder.new()
    :named("final")
    :prefix_with("final")
    :decorate(function(note: string, _state: protocol.BusState, event: protocol.Event): string
        if event.kind == "completed" then
            return note .. ":" .. event.output
        end
        if event.kind == "failed" then
            return note .. ":" .. event.code
        end
        return note
    end)
    :build()

local projector = projector_builder.new()
    :track_queue("priority")
    :count_failures_as("failed")
    :capture_source("source")
    :build()

local app = bus.new()
    :register_projector("tasks", projector)
    :register_subscriber("tasks", source_subscriber)
    :register_subscriber("tasks", final_subscriber)

app:on_step(function(step: protocol.DispatchStep, state: protocol.BusState)
    last_bus_id = state.id

    if step.kind == "subscriber" then
        observed_notes[step.topic .. ":" .. tostring(#observed_audits + 1)] = step.note
        if step.projection_id then
            local projection_id: string = step.projection_id
        end
    else
        table.insert(observed_audits, step.note)
        local at_seconds: integer = step.at:unix()
    end
end)

local queued_one: protocol.TaskQueuedEvent = {
    kind = "queued",
    id = "job-1",
    queue = "priority",
    payload = {task = "search"},
    meta = protocol.meta("trace-1", {source = "api", lane = "priority"}),
}

local started_one: protocol.TaskStartedEvent = {
    kind = "started",
    id = "job-1",
    worker = "worker-a",
    meta = protocol.meta("trace-2", {source = "worker"}),
}

local completed_one: protocol.TaskCompletedEvent = {
    kind = "completed",
    id = "job-1",
    output = "done",
    meta = protocol.meta("trace-3", {source = "worker"}),
}

local queued_two: protocol.TaskQueuedEvent = {
    kind = "queued",
    id = "job-2",
    queue = "priority",
    payload = {task = "profile"},
    meta = protocol.meta("trace-4", {source = "api"}),
}

local failed_two: protocol.TaskFailedEvent = {
    kind = "failed",
    id = "job-2",
    code = "rate_limited",
    retryable = true,
    meta = protocol.meta("trace-5", {source = "worker"}),
}

local tick: protocol.TickEvent = {
    kind = "tick",
    at = now,
}

local events: {protocol.Event} = {
    queued_one,
    started_one,
    completed_one,
    queued_two,
    failed_two,
    tick,
}

local store = app:new_store("bus-1", now)
store.state.flags["replayed"] = true

local summary_result = app:replay(store, "tasks", events, now)
if not summary_result.ok then
    local message: string = summary_result.error.message
    local retryable: boolean = summary_result.error.retryable
else
    local summary = summary_result.value
    local bus_id: string = summary.id
    local total_steps: number = summary.total_steps
    local projection_count: number = summary.projection_count
    local failure_count: number = summary.failure_count
    local elapsed_seconds: number = summary.elapsed_seconds
    local last_status: string? = summary.last_status
end

local summary_label = result.map(summary_result, function(summary: protocol.DispatchSummary): string
    return summary.id .. ":" .. tostring(summary.projection_count)
end)

if summary_label.ok then
    local label: string = summary_label.value
end

local summary_id = result.and_then(summary_result, function(summary: protocol.DispatchSummary): StringResult
    if summary.failure_count == 0 then
        return {
            ok = false,
            error = {
                code = "invalid",
                message = "expected a failure",
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

local projection_one = store:lookup_projection("job-1")
if projection_one then
    local status: string = projection_one.status
    local worker = projection_one.worker
    if worker then
        local stable_worker: string = worker
    end
    local output = projection_one.output
    if output then
        local stable_output: string = output
    end
    local source = projection_one.source
    if source then
        local stable_source: string = source
    end
end

local projection_two = store:lookup_projection("job-2")
if projection_two then
    local failed_status: string = projection_two.status
    local error_code = projection_two.error_code
    if error_code then
        local stable_code: string = error_code
    end
    local retryable = projection_two.retryable
    if retryable ~= nil then
        local stable_retryable: boolean = retryable
    end
end

local missing = store:lookup_projection("missing")
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

if last_bus_id ~= nil then
    local stable_bus_id: string = last_bus_id
end

local event_source = helpers.source_tag(queued_one)
if event_source then
    local stable_event_source: string = event_source
end

local seen_at = store.state.last_event_at or store.state.started_at
local elapsed = now:sub(seen_at)
local seconds: number = elapsed:seconds()
