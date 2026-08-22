local time = require("time")
local protocol = require("protocol")

type OrderStore = {
    state: protocol.StoreState,
    touch: (self: OrderStore, at: time.Time) -> OrderStore,
    push_step: (self: OrderStore, step: protocol.RunStep, at: time.Time) -> OrderStore,
    ensure_order: (self: OrderStore, id: string, customer: string, at: time.Time) -> protocol.OrderAggregate,
    lookup_order: (self: OrderStore, id: string) -> protocol.OrderAggregate?,
    ensure_view: (self: OrderStore, id: string, status: "created" | "reserved" | "completed") -> protocol.OrderView,
    lookup_view: (self: OrderStore, id: string) -> protocol.OrderView?,
    increment: (self: OrderStore, name: string) -> integer,
    summarize: (self: OrderStore, now: time.Time, last_status: string?) -> protocol.RunSummary,
}

type Store = OrderStore

local Store = {}
Store.__index = Store

local M = {}
M.OrderStore = OrderStore

function M.new(id: string, now: time.Time): OrderStore
    local self: Store = {
        state = {
            id = id,
            started_at = now,
            last_command_at = nil,
            steps = {},
            orders = {},
            views = {},
            counters = {},
            flags = {},
        },
        touch = Store.touch,
        push_step = Store.push_step,
        ensure_order = Store.ensure_order,
        lookup_order = Store.lookup_order,
        ensure_view = Store.ensure_view,
        lookup_view = Store.lookup_view,
        increment = Store.increment,
        summarize = Store.summarize,
    }
    setmetatable(self, Store)
    return self
end

function Store:touch(at: time.Time): Store
    self.state.last_command_at = at
    return self
end

function Store:push_step(step: protocol.RunStep, at: time.Time): Store
    table.insert(self.state.steps, step)
    return self:touch(at)
end

function Store:ensure_order(id: string, customer: string, at: time.Time): protocol.OrderAggregate
    local current = self.state.orders[id]
    if current then
        if current.updated_at == nil then
            current.updated_at = at
        end
        return current
    end

    local created: protocol.OrderAggregate = {
        id = id,
        customer = customer,
        version = 0,
        status = "created",
        item_id = nil,
        source = nil,
        updated_at = at,
    }
    self.state.orders[id] = created
    return created
end

function Store:lookup_order(id: string): protocol.OrderAggregate?
    return self.state.orders[id]
end

function Store:ensure_view(id: string, status: "created" | "reserved" | "completed"): protocol.OrderView
    local current = self.state.views[id]
    if current then
        return current
    end

    local created: protocol.OrderView = {
        id = id,
        status = status,
        version = 0,
        item_id = nil,
        source = nil,
        completed_at = nil,
    }
    self.state.views[id] = created
    return created
end

function Store:lookup_view(id: string): protocol.OrderView?
    return self.state.views[id]
end

function Store:increment(name: string): integer
    local current = self.state.counters[name] or 0
    local next_value = current + 1
    self.state.counters[name] = next_value
    return next_value
end

function Store:summarize(now: time.Time, last_status: string?): protocol.RunSummary
    local order_count = 0
    local completed_count = 0
    for _, view in pairs(self.state.views) do
        order_count = order_count + 1
        if view.status == "completed" then
            completed_count = completed_count + 1
        end
    end

    local seen_at = self.state.last_command_at or self.state.started_at
    local elapsed = now:sub(seen_at)

    return {
        id = self.state.id,
        total_steps = #self.state.steps,
        order_count = order_count,
        completed_count = completed_count,
        last_status = last_status,
        elapsed_seconds = elapsed:seconds(),
    }
end

return M
