local time = require("time")
local protocol = require("protocol")
local runtime_store = require("runtime_store")

type Runtime = {
    handlers: {[string]: protocol.PluginHandler},
    hooks: {protocol.Hook},
    register_plugin: (self: Runtime, name: string, handler: protocol.PluginHandler) -> Runtime,
    on_step: (self: Runtime, hook: protocol.Hook) -> Runtime,
    new_store: (self: Runtime, id: string, now: time.Time) -> runtime_store.RuntimeStore,
}

local Runtime = {}
Runtime.__index = Runtime

local M = {}

function M.new(): Runtime
    local self: Runtime = {
        handlers = {},
        hooks = {},
        register_plugin = Runtime.register_plugin,
        on_step = Runtime.on_step,
        new_store = Runtime.new_store,
    }
    setmetatable(self, Runtime)
    return self
end

function Runtime:register_plugin(name: string, handler: protocol.PluginHandler): Runtime
    self.handlers[name] = handler
    return self
end

function Runtime:on_step(hook: protocol.Hook): Runtime
    table.insert(self.hooks, hook)
    return self
end

function Runtime:new_store(id: string, now: time.Time): runtime_store.RuntimeStore
    return runtime_store.new(id, now)
end

return M
