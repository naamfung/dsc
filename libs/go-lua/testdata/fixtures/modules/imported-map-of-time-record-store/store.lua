local time = require("time")
local protocol = require("protocol")

type Store = {
    sessions: {[string]: protocol.Snapshot},
    open: (self: Store, id: string, now: time.Time) -> protocol.Snapshot,
    get: (self: Store, id: string) -> protocol.Snapshot?,
}

local Store = {}
Store.__index = Store

local M = {}
M.Store = Store

function M.new(): Store
    local self: Store = {
        sessions = {},
        open = Store.open,
        get = Store.get,
    }
    setmetatable(self, Store)
    return self
end

function Store:open(id: string, now: time.Time): protocol.Snapshot
    local existing = self.sessions[id]
    if existing then
        existing.last_seen = now
        return existing
    end

    local created: protocol.Snapshot = {
        id = id,
        opened_at = now,
        last_seen = now,
        last_value = nil,
        flags = {},
    }
    self.sessions[id] = created
    return created
end

function Store:get(id: string): protocol.Snapshot?
    return self.sessions[id]
end

return M
