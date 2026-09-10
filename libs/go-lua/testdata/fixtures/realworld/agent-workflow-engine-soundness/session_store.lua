local time = require("time")
local protocol = require("protocol")

type SessionStore = {
    state: protocol.SessionState,
    lookup_tool: (self: SessionStore, name: string) -> protocol.ToolResult?,
}

type Store = SessionStore

local Store = {}
Store.__index = Store

local M = {}
M.SessionStore = SessionStore

function M.new(id: string, now: time.Time): SessionStore
    local self: Store = {
        state = {
            id = id,
            started_at = now,
            last_activity = nil,
            tool_cache = {},
        },
        lookup_tool = Store.lookup_tool,
    }
    setmetatable(self, Store)
    return self
end

function Store:lookup_tool(name: string): protocol.ToolResult?
    return self.state.tool_cache[name]
end

return M
