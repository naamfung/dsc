local time = require("time")
local protocol = require("protocol")

type CheckpointStore = {
    state: protocol.BusState,
    touch: (self: CheckpointStore, at: time.Time) -> CheckpointStore,
    push_step: (self: CheckpointStore, step: protocol.DispatchStep, at: time.Time) -> CheckpointStore,
    ensure_projection: (self: CheckpointStore, id: string, queue: string, at: time.Time) -> protocol.TaskProjection,
    lookup_projection: (self: CheckpointStore, id: string) -> protocol.TaskProjection?,
    increment: (self: CheckpointStore, name: string) -> integer,
    summarize: (self: CheckpointStore, now: time.Time, last_status: string?) -> protocol.DispatchSummary,
}

type Store = CheckpointStore

local Store = {}
Store.__index = Store

local M = {}
M.CheckpointStore = CheckpointStore

function M.new(id: string, now: time.Time): CheckpointStore
    local self: Store = {
        state = {
            id = id,
            started_at = now,
            last_event_at = nil,
            steps = {},
            projections = {},
            counters = {},
            flags = {},
        },
        touch = Store.touch,
        push_step = Store.push_step,
        ensure_projection = Store.ensure_projection,
        lookup_projection = Store.lookup_projection,
        increment = Store.increment,
        summarize = Store.summarize,
    }
    setmetatable(self, Store)
    return self
end

function Store:touch(at: time.Time): Store
    self.state.last_event_at = at
    return self
end

function Store:push_step(step: protocol.DispatchStep, at: time.Time): Store
    table.insert(self.state.steps, step)
    return self:touch(at)
end

function Store:ensure_projection(id: string, queue: string, at: time.Time): protocol.TaskProjection
    local current = self.state.projections[id]
    if current then
        if current.updated_at == nil then
            current.updated_at = at
        end
        if current.queue == "unknown" and queue ~= "unknown" then
            current.queue = queue
        end
        return current
    end

    local created: protocol.TaskProjection = {
        id = id,
        queue = queue,
        status = "queued",
        worker = nil,
        output = nil,
        error_code = nil,
        retryable = nil,
        source = nil,
        updated_at = at,
    }
    self.state.projections[id] = created
    return created
end

function Store:lookup_projection(id: string): protocol.TaskProjection?
    return self.state.projections[id]
end

function Store:increment(name: string): integer
    local current = self.state.counters[name] or 0
    local next_value = current + 1
    self.state.counters[name] = next_value
    return next_value
end

function Store:summarize(now: time.Time, last_status: string?): protocol.DispatchSummary
    local projection_count = 0
    for _, _ in pairs(self.state.projections) do
        projection_count = projection_count + 1
    end

    local failure_count = self.state.counters["failed"] or 0
    local seen_at = self.state.last_event_at or self.state.started_at
    local elapsed = now:sub(seen_at)

    return {
        id = self.state.id,
        total_steps = #self.state.steps,
        projection_count = projection_count,
        failure_count = failure_count,
        last_status = last_status,
        elapsed_seconds = elapsed:seconds(),
    }
end

return M
