local time = require("time")
local protocol = require("protocol")

type SessionStore = {
    state: protocol.SessionState,
    touch: (self: SessionStore, at: time.Time) -> SessionStore,
    append_message: (self: SessionStore, msg: protocol.Message, at: time.Time) -> SessionStore,
    emit_step: (self: SessionStore, step: protocol.WorkflowStep, at: time.Time) -> SessionStore,
    remember_tool: (self: SessionStore, tool: protocol.ToolResult) -> (),
    lookup_tool: (self: SessionStore, name: string) -> protocol.ToolResult?,
    mark_flag: (self: SessionStore, name: string) -> (),
    summarize: (self: SessionStore, now: time.Time, last_reason: string?) -> protocol.SessionSummary,
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
            messages = {},
            steps = {},
            flags = {},
            tool_cache = {},
        },
        touch = Store.touch,
        append_message = Store.append_message,
        emit_step = Store.emit_step,
        remember_tool = Store.remember_tool,
        lookup_tool = Store.lookup_tool,
        mark_flag = Store.mark_flag,
        summarize = Store.summarize,
    }
    setmetatable(self, Store)
    return self
end

function Store:touch(at: time.Time): Store
    self.state.last_activity = at
    return self
end

function Store:append_message(msg: protocol.Message, at: time.Time): Store
    table.insert(self.state.messages, msg)
    return self:touch(at)
end

function Store:emit_step(step: protocol.WorkflowStep, at: time.Time): Store
    table.insert(self.state.steps, step)
    return self:touch(at)
end

function Store:remember_tool(tool: protocol.ToolResult)
    self.state.tool_cache[tool.tool] = tool
end

function Store:lookup_tool(name: string): protocol.ToolResult?
    return self.state.tool_cache[name]
end

function Store:mark_flag(name: string)
    self.state.flags[name] = true
end

function Store:summarize(now: time.Time, last_reason: string?): protocol.SessionSummary
    local cached_tool_count = 0
    for _, _ in pairs(self.state.tool_cache) do
        cached_tool_count = cached_tool_count + 1
    end

    local since = self.state.last_activity or self.state.started_at
    local latency = now:sub(since)

    return {
        id = self.state.id,
        total_steps = #self.state.steps,
        cached_tool_count = cached_tool_count,
        last_latency_seconds = latency:seconds(),
        last_reason = last_reason,
    }
end

return M
