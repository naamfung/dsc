local time = require("time")
local protocol = require("protocol")

type SessionStore = {
    by_token: {[string]: protocol.SessionSnapshot},
    lookup: (self: SessionStore, token: string) -> protocol.SessionSnapshot?,
    touch: (self: SessionStore, token: string, at: time.Time) -> (),
    save: (self: SessionStore, token: string, snapshot: protocol.SessionSnapshot) -> (),
}

type Store = SessionStore

local Store = {}
Store.__index = Store

local M = {}
M.SessionStore = SessionStore

function M.new(): SessionStore
    local self: Store = {
        by_token = {},
        lookup = Store.lookup,
        touch = Store.touch,
        save = Store.save,
    }
    setmetatable(self, Store)
    return self
end

function Store:lookup(token: string): protocol.SessionSnapshot?
    return self.by_token[token]
end

function Store:touch(token: string, at: time.Time)
    local snapshot = self.by_token[token]
    if snapshot then
        snapshot.last_seen = at
    end
end

function Store:save(token: string, snapshot: protocol.SessionSnapshot)
    self.by_token[token] = snapshot
end

return M
