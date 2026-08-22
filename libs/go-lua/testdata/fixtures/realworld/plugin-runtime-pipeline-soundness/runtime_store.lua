local time = require("time")
local protocol = require("protocol")

type RuntimeStore = {
    state: protocol.RuntimeState,
    lookup: (self: RuntimeStore, name: string) -> protocol.PluginOutput?,
}

type Store = RuntimeStore

local Store = {}
Store.__index = Store

local M = {}
M.RuntimeStore = RuntimeStore

function M.new(id: string, now: time.Time): RuntimeStore
    local self: Store = {
        state = {
            id = id,
            started_at = now,
            last_seen = nil,
            steps = {},
            cache = {},
            flags = {},
        },
        lookup = Store.lookup,
    }
    setmetatable(self, Store)
    return self
end

function Store:lookup(name: string): protocol.PluginOutput?
    return self.state.cache[name]
end

return M
