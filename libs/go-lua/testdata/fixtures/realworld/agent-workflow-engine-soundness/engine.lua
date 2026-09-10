local time = require("time")
local protocol = require("protocol")
local session_store = require("session_store")

type Engine = {
    handlers: {[string]: protocol.ToolHandler},
    register_tool: (self: Engine, name: string, handler: protocol.ToolHandler) -> Engine,
    new_session: (self: Engine, id: string, now: time.Time) -> session_store.SessionStore,
}

local Engine = {}
Engine.__index = Engine

local M = {}

function M.new(): Engine
    local self: Engine = {
        handlers = {},
        register_tool = Engine.register_tool,
        new_session = Engine.new_session,
    }
    setmetatable(self, Engine)
    return self
end

function Engine:register_tool(name: string, handler: protocol.ToolHandler): Engine
    self.handlers[name] = handler
    return self
end

function Engine:new_session(id: string, now: time.Time): session_store.SessionStore
    return session_store.new(id, now)
end

return M
