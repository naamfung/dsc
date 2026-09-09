local time = require("time")
local protocol = require("protocol")

type SagaStore = {
    state: protocol.StoreState,
    touch: (self: SagaStore, at: time.Time) -> SagaStore,
    push_step: (self: SagaStore, step: protocol.SagaStep, at: time.Time) -> SagaStore,
    lookup_saga: (self: SagaStore, order_id: string) -> protocol.SagaAggregate?,
    lookup_view: (self: SagaStore, order_id: string) -> protocol.SagaView?,
    increment: (self: SagaStore, name: string) -> integer,
    summarize: (self: SagaStore, now: time.Time, last_status: string?) -> protocol.RunSummary,
}

type Store = SagaStore

local Store = {}
Store.__index = Store

local M = {}
M.SagaStore = SagaStore

function M.new(id: string, now: time.Time): SagaStore
    local self: Store = {
        state = {
            id = id,
            started_at = now,
            last_action_at = nil,
            steps = {},
            sagas = {},
            views = {},
            counters = {},
            flags = {},
        },
        touch = Store.touch,
        push_step = Store.push_step,
        lookup_saga = Store.lookup_saga,
        lookup_view = Store.lookup_view,
        increment = Store.increment,
        summarize = Store.summarize,
    }
    setmetatable(self, Store)
    return self
end

function Store:touch(at: time.Time): Store
    self.state.last_action_at = at
    return self
end

function Store:push_step(step: protocol.SagaStep, at: time.Time): Store
    table.insert(self.state.steps, step)
    return self:touch(at)
end

function Store:lookup_saga(order_id: string): protocol.SagaAggregate?
    return self.state.sagas[order_id]
end

function Store:lookup_view(order_id: string): protocol.SagaView?
    return self.state.views[order_id]
end

function Store:increment(name: string): integer
    local current = self.state.counters[name] or 0
    local next_value = current + 1
    self.state.counters[name] = next_value
    return next_value
end

function Store:summarize(now: time.Time, last_status: string?): protocol.RunSummary
    local saga_count = 0
    local committed_count = 0
    local rolled_back_count = 0
    for _, view in pairs(self.state.views) do
        saga_count = saga_count + 1
        if view.status == "committed" then
            committed_count = committed_count + 1
        elseif view.status == "rolled_back" then
            rolled_back_count = rolled_back_count + 1
        end
    end

    local seen_at = self.state.last_action_at or self.state.started_at
    local elapsed = now:sub(seen_at)

    return {
        id = self.state.id,
        total_steps = #self.state.steps,
        saga_count = saga_count,
        committed_count = committed_count,
        rolled_back_count = rolled_back_count,
        last_status = last_status,
        elapsed_seconds = elapsed:seconds(),
    }
end

return M
