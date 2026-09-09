local time = require("time")
local result = require("result")

type AppError = result.AppError

type EventMeta = {
    trace_id: string,
    tags: {[string]: string}?,
}

type TaskQueuedEvent = {
    kind: "queued",
    id: string,
    queue: string,
    payload: {[string]: string},
    meta: EventMeta,
}

type TaskStartedEvent = {
    kind: "started",
    id: string,
    worker: string,
    meta: EventMeta,
}

type TaskCompletedEvent = {
    kind: "completed",
    id: string,
    output: string,
    meta: EventMeta,
}

type TaskFailedEvent = {
    kind: "failed",
    id: string,
    code: string,
    retryable: boolean,
    meta: EventMeta,
}

type TickEvent = {
    kind: "tick",
    at: time.Time,
}

type Event = TaskQueuedEvent | TaskStartedEvent | TaskCompletedEvent | TaskFailedEvent | TickEvent

type TaskProjection = {
    id: string,
    queue: string,
    status: "queued" | "started" | "completed" | "failed",
    worker: string?,
    output: string?,
    error_code: string?,
    retryable: boolean?,
    source: string?,
    updated_at: time.Time?,
}

type SubscriberStep = {
    kind: "subscriber",
    topic: string,
    note: string,
    projection_id: string?,
}

type AuditStep = {
    kind: "audit",
    note: string,
    at: time.Time,
}

type DispatchStep = SubscriberStep | AuditStep

type BusState = {
    id: string,
    started_at: time.Time,
    last_event_at: time.Time?,
    steps: {DispatchStep},
    projections: {[string]: TaskProjection},
    counters: {[string]: integer},
    flags: {[string]: boolean},
}

type DispatchSummary = {
    id: string,
    total_steps: number,
    projection_count: number,
    failure_count: number,
    last_status: string?,
    elapsed_seconds: number,
}

type SubscriberResult = {ok: true, value: string?} | {ok: false, error: AppError}
type PublishResult = {ok: true, value: string?} | {ok: false, error: AppError}
type ReplayResult = {ok: true, value: DispatchSummary} | {ok: false, error: AppError}

type Subscriber = (BusState, Event) -> SubscriberResult
type Projector = (BusState, Event, time.Time) -> ()
type StepHook = (DispatchStep, BusState) -> ()

local M = {}
M.AppError = AppError
M.EventMeta = EventMeta
M.TaskQueuedEvent = TaskQueuedEvent
M.TaskStartedEvent = TaskStartedEvent
M.TaskCompletedEvent = TaskCompletedEvent
M.TaskFailedEvent = TaskFailedEvent
M.TickEvent = TickEvent
M.Event = Event
M.TaskProjection = TaskProjection
M.DispatchStep = DispatchStep
M.BusState = BusState
M.DispatchSummary = DispatchSummary
M.SubscriberResult = SubscriberResult
M.PublishResult = PublishResult
M.ReplayResult = ReplayResult
M.Subscriber = Subscriber
M.Projector = Projector
M.StepHook = StepHook

function M.meta(trace_id: string, tags: {[string]: string}?): EventMeta
    return {
        trace_id = trace_id,
        tags = tags,
    }
end

return M
