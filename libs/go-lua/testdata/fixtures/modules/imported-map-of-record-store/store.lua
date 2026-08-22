local protocol = require("protocol")

type Store = {
    sessions: {[string]: protocol.Snapshot},
    put: (self: Store, id: string, snapshot: protocol.Snapshot) -> (),
    get: (self: Store, id: string) -> protocol.Snapshot?,
}

local Store = {}
Store.__index = Store

local M = {}
M.Store = Store

function M.new(): Store
    local self: Store = {
        sessions = {},
        put = Store.put,
        get = Store.get,
    }
    setmetatable(self, Store)
    return self
end

function Store:put(id: string, snapshot: protocol.Snapshot)
    self.sessions[id] = snapshot
end

function Store:get(id: string): protocol.Snapshot?
    return self.sessions[id]
end

return M
