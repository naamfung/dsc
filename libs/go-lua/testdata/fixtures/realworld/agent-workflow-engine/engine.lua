local time = require("time")
local result = require("result")
local protocol = require("protocol")
local session_store = require("session_store")

type AppError = result.AppError
type StepResult = {ok: true, value: string?} | {ok: false, error: AppError}
type SummaryResult = {ok: true, value: protocol.SessionSummary} | {ok: false, error: AppError}

type Engine = {
    handlers: {[string]: protocol.ToolHandler},
    listeners: {protocol.StepListener},
    register_tool: (self: Engine, name: string, handler: protocol.ToolHandler) -> Engine,
    on_step: (self: Engine, listener: protocol.StepListener) -> Engine,
    emit: (self: Engine, store: session_store.SessionStore, step: protocol.WorkflowStep, at: time.Time) -> (),
    new_session: (self: Engine, id: string, now: time.Time) -> session_store.SessionStore,
    process_message: (self: Engine, store: session_store.SessionStore, msg: protocol.Message, at: time.Time) -> StepResult,
    process: (self: Engine, store: session_store.SessionStore, messages: {protocol.Message}, now: time.Time) -> SummaryResult,
}

local Engine = {}
Engine.__index = Engine

local M = {}
M.Engine = Engine

function M.new(): Engine
    local self: Engine = {
        handlers = {},
        listeners = {},
        register_tool = Engine.register_tool,
        on_step = Engine.on_step,
        emit = Engine.emit,
        new_session = Engine.new_session,
        process_message = Engine.process_message,
        process = Engine.process,
    }
    setmetatable(self, Engine)
    return self
end

function Engine:register_tool(name: string, handler: protocol.ToolHandler): Engine
    self.handlers[name] = handler
    return self
end

function Engine:on_step(listener: protocol.StepListener): Engine
    table.insert(self.listeners, listener)
    return self
end

function Engine:emit(store: session_store.SessionStore, step: protocol.WorkflowStep, at: time.Time)
    store:emit_step(step, at)
    for _, listener in ipairs(self.listeners) do
        listener(step, store.state)
    end
end

function Engine:new_session(id: string, now: time.Time): session_store.SessionStore
    return session_store.new(id, now)
end

function Engine:process_message(store: session_store.SessionStore, msg: protocol.Message, at: time.Time): StepResult
    store:append_message(msg, at)

    if msg.kind == "user" then
        self:emit(store, {kind = "assistant", content = "ack:" .. msg.content}, at)
        return {ok = true, value = nil}
    end

    if msg.kind == "tool_call" then
        local cached = store:lookup_tool(msg.tool)
        if cached then
            self:emit(store, {
                kind = "tool",
                tool = msg.tool,
                result = {
                    tool = cached.tool,
                    content = cached.content,
                    cached = true,
                },
            }, at)
            store:mark_flag("cache_hit")
            return {ok = true, value = nil}
        end

        local handler = self.handlers[msg.tool]
        if not handler then
            return {
                ok = false,
                error = {
                    code = "not_found",
                    message = "missing tool handler: " .. msg.tool,
                    retryable = false,
                },
            }
        end

        local tool_result = handler(store.state, msg)
        if tool_result.ok then
            store:remember_tool(tool_result.value)
            self:emit(store, {
                kind = "tool",
                tool = msg.tool,
                result = tool_result.value,
            }, at)

            if msg.tool == "profile" then
                store:mark_flag("profile_loaded")
            end
            return {ok = true, value = nil}
        end

        return {
            ok = false,
            error = {
                code = tool_result.error.code,
                message = tool_result.error.message,
                retryable = tool_result.error.retryable,
            },
        }
    end

    self:emit(store, {kind = "audit", note = "done:" .. msg.reason, at = at}, at)
    return {ok = true, value = msg.reason}
end

function Engine:process(
    store: session_store.SessionStore,
    messages: {protocol.Message},
    now: time.Time
): SummaryResult
    local last_reason: string? = nil

    for _, msg in ipairs(messages) do
        local step_result = self:process_message(store, msg, now)
        if not step_result.ok then
            return step_result
        end
        if step_result.value ~= nil then
            last_reason = step_result.value
        end
    end

    return {
        ok = true,
        value = store:summarize(now, last_reason),
    }
end

return M
