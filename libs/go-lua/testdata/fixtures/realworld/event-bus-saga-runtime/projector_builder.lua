local protocol = require("protocol")

type ProjectorBuilder = {
    tracked_queue: string?,
    failure_counter: string?,
    source_key: string?,
    track_queue: (self: ProjectorBuilder, queue: string) -> ProjectorBuilder,
    count_failures_as: (self: ProjectorBuilder, counter_name: string) -> ProjectorBuilder,
    capture_source: (self: ProjectorBuilder, tag_key: string) -> ProjectorBuilder,
    build: (self: ProjectorBuilder) -> protocol.Projector,
}

type Builder = ProjectorBuilder

local Builder = {}
Builder.__index = Builder

local M = {}
M.ProjectorBuilder = ProjectorBuilder

function M.new(): ProjectorBuilder
    local self: Builder = {
        tracked_queue = nil,
        failure_counter = nil,
        source_key = nil,
        track_queue = Builder.track_queue,
        count_failures_as = Builder.count_failures_as,
        capture_source = Builder.capture_source,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:track_queue(queue: string): Builder
    self.tracked_queue = queue
    return self
end

function Builder:count_failures_as(counter_name: string): Builder
    self.failure_counter = counter_name
    return self
end

function Builder:capture_source(tag_key: string): Builder
    self.source_key = tag_key
    return self
end

function Builder:build(): protocol.Projector
    local tracked_queue = self.tracked_queue
    local failure_counter = self.failure_counter
    local source_key = self.source_key

    return function(state: protocol.BusState, event: protocol.Event, at)
        if event.kind == "tick" then
            return
        end

        local queue_name = "unknown"
        if event.kind == "queued" then
            queue_name = event.queue
        end

        if tracked_queue and event.kind == "queued" and event.queue ~= tracked_queue then
            return
        end

        local projection = state.projections[event.id]
        if not projection then
            projection = {
                id = event.id,
                queue = queue_name,
                status = "queued",
                worker = nil,
                output = nil,
                error_code = nil,
                retryable = nil,
                source = nil,
                updated_at = at,
            }
            state.projections[event.id] = projection
        end

        if projection.queue == "unknown" and queue_name ~= "unknown" then
            projection.queue = queue_name
        end

        local tags = event.meta.tags
        if source_key and tags then
            local source = tags[source_key]
            if source then
                projection.source = source
            end
        end

        if event.kind == "queued" then
            projection.status = "queued"
        elseif event.kind == "started" then
            projection.status = "started"
            projection.worker = event.worker
        elseif event.kind == "completed" then
            projection.status = "completed"
            projection.output = event.output
        elseif event.kind == "failed" then
            projection.status = "failed"
            projection.error_code = event.code
            projection.retryable = event.retryable
            if failure_counter then
                local current = state.counters[failure_counter] or 0
                state.counters[failure_counter] = current + 1
            end
        end

        projection.updated_at = at
    end
end

return M
