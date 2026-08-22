local time = require("time")
local result = require("result")
local protocol = require("protocol")
local runtime_store = require("runtime_store")

type EventResult = {ok: true, value: string?} | {ok: false, error: result.AppError}

type Runtime = {
    handlers: {[string]: protocol.PluginHandler},
    hooks: {protocol.Hook},
    default_policy: protocol.RetryPolicy,
    register_plugin: (self: Runtime, name: string, handler: protocol.PluginHandler) -> Runtime,
    on_step: (self: Runtime, hook: protocol.Hook) -> Runtime,
    new_store: (self: Runtime, id: string, now: time.Time) -> runtime_store.RuntimeStore,
    emit: (self: Runtime, store: runtime_store.RuntimeStore, step: protocol.RuntimeStep, at: time.Time) -> (),
    handle_event: (self: Runtime, store: runtime_store.RuntimeStore, event: protocol.Event, at: time.Time) -> EventResult,
    run: (self: Runtime, store: runtime_store.RuntimeStore, events: {protocol.Event}, now: time.Time) -> protocol.RuntimeResult,
}

local Runtime = {}
Runtime.__index = Runtime

local M = {}
M.Runtime = Runtime

function M.new(policy: protocol.RetryPolicy): Runtime
    local self: Runtime = {
        handlers = {},
        hooks = {},
        default_policy = policy,
        register_plugin = Runtime.register_plugin,
        on_step = Runtime.on_step,
        new_store = Runtime.new_store,
        emit = Runtime.emit,
        handle_event = Runtime.handle_event,
        run = Runtime.run,
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

function Runtime:emit(store: runtime_store.RuntimeStore, step: protocol.RuntimeStep, at: time.Time)
    store:push_step(step, at)
    for _, hook in ipairs(self.hooks) do
        hook(step, store.state)
    end
end

function Runtime:handle_event(store: runtime_store.RuntimeStore, event: protocol.Event, at: time.Time): EventResult
    if event.kind == "heartbeat" then
        self:emit(store, {kind = "audit", note = "heartbeat", at = event.at}, at)
        return {ok = true, value = nil}
    end

    if event.kind == "done" then
        local reason = event.reason or event.status
        self:emit(store, {kind = "audit", note = "done:" .. reason, at = at}, at)
        return {ok = true, value = event.status}
    end

    local cached = store:lookup(event.plugin)
    if cached then
        self:emit(store, {
            kind = "plugin",
            plugin = event.plugin,
            output = {
                plugin = cached.plugin,
                content = cached.content,
                cached = true,
                tags = cached.tags,
            },
        }, at)
        store:set_flag("cache_hit")
        return {ok = true, value = nil}
    end

    local handler = self.handlers[event.plugin]
    if not handler then
        return {
            ok = false,
            error = {
                code = "not_found",
                message = "missing plugin: " .. event.plugin,
                retryable = false,
            },
        }
    end

    local plugin_result = handler(store.state, event, self.default_policy)
    if plugin_result.ok then
        store:remember(plugin_result.value)
        self:emit(store, {
            kind = "plugin",
            plugin = event.plugin,
            output = plugin_result.value,
        }, at)
        return {ok = true, value = nil}
    end

    local detail = "stop"
    if self.default_policy.should_retry(plugin_result.error, 1) then
        detail = "retry"
    end
    self:emit(store, {kind = "hook", name = "retry", detail = detail}, at)
    return {ok = false, error = plugin_result.error}
end

function Runtime:run(
    store: runtime_store.RuntimeStore,
    events: {protocol.Event},
    now: time.Time
): protocol.RuntimeResult
    local last_status: string? = nil

    for _, event in ipairs(events) do
        local event_result = self:handle_event(store, event, now)
        if not event_result.ok then
            return {ok = false, error = event_result.error}
        end
        if event_result.value ~= nil then
            last_status = event_result.value
        end
    end

    return {
        ok = true,
        value = store:summarize(now, last_status),
    }
end

return M
