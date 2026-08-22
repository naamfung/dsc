local time = require("time")
local protocol = require("protocol")

type RuntimeStore = {
    state: protocol.RuntimeState,
    touch: (self: RuntimeStore, at: time.Time) -> RuntimeStore,
    push_step: (self: RuntimeStore, step: protocol.RuntimeStep, at: time.Time) -> RuntimeStore,
    remember: (self: RuntimeStore, output: protocol.PluginOutput) -> (),
    lookup: (self: RuntimeStore, name: string) -> protocol.PluginOutput?,
    set_flag: (self: RuntimeStore, name: string) -> (),
    summarize: (self: RuntimeStore, now: time.Time, last_status: string?) -> protocol.RuntimeSummary,
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
        touch = Store.touch,
        push_step = Store.push_step,
        remember = Store.remember,
        lookup = Store.lookup,
        set_flag = Store.set_flag,
        summarize = Store.summarize,
    }
    setmetatable(self, Store)
    return self
end

function Store:touch(at: time.Time): Store
    self.state.last_seen = at
    return self
end

function Store:push_step(step: protocol.RuntimeStep, at: time.Time): Store
    table.insert(self.state.steps, step)
    return self:touch(at)
end

function Store:remember(output: protocol.PluginOutput)
    self.state.cache[output.plugin] = output
end

function Store:lookup(name: string): protocol.PluginOutput?
    return self.state.cache[name]
end

function Store:set_flag(name: string)
    self.state.flags[name] = true
end

function Store:summarize(now: time.Time, last_status: string?): protocol.RuntimeSummary
    local cached_count = 0
    local last_plugin: string? = nil
    for name, _ in pairs(self.state.cache) do
        cached_count = cached_count + 1
        last_plugin = name
    end

    local since = self.state.last_seen or self.state.started_at
    local elapsed = now:sub(since)

    return {
        id = self.state.id,
        total_steps = #self.state.steps,
        cached_count = cached_count,
        last_status = last_status,
        elapsed_seconds = elapsed:seconds(),
        last_plugin = last_plugin,
    }
end

return M
