local time = require("time")
local protocol = require("protocol")
local helpers = require("helpers")
local plugin_store = require("plugin_store")

type SupervisorRuntime = {
    validators: {protocol.RequestValidator},
    handlers: {[string]: protocol.PluginHandler},
    fallbacks: {protocol.FallbackHandler},
    hooks: {protocol.StepHook},
    use_validator: (self: SupervisorRuntime, validator: protocol.RequestValidator) -> SupervisorRuntime,
    register_handler: (self: SupervisorRuntime, plugin_name: string, handler: protocol.PluginHandler) -> SupervisorRuntime,
    use_fallback: (self: SupervisorRuntime, fallback: protocol.FallbackHandler) -> SupervisorRuntime,
    on_step: (self: SupervisorRuntime, hook: protocol.StepHook) -> SupervisorRuntime,
    new_store: (self: SupervisorRuntime, id: string, now: time.Time) -> plugin_store.PluginStore,
    emit: (self: SupervisorRuntime, store: plugin_store.PluginStore, step: protocol.RuntimeStep, at: time.Time) -> (),
    dispatch: (self: SupervisorRuntime, store: plugin_store.PluginStore, request: protocol.Request, at: time.Time) -> protocol.DispatchResult,
    run: (self: SupervisorRuntime, store: plugin_store.PluginStore, requests: {protocol.Request}, now: time.Time) -> protocol.RunResult,
}

type Runtime = SupervisorRuntime

local Runtime = {}
Runtime.__index = Runtime

local M = {}
M.SupervisorRuntime = SupervisorRuntime

function M.new(): SupervisorRuntime
    local self: Runtime = {
        validators = {},
        handlers = {},
        fallbacks = {},
        hooks = {},
        use_validator = Runtime.use_validator,
        register_handler = Runtime.register_handler,
        use_fallback = Runtime.use_fallback,
        on_step = Runtime.on_step,
        new_store = Runtime.new_store,
        emit = Runtime.emit,
        dispatch = Runtime.dispatch,
        run = Runtime.run,
    }
    setmetatable(self, Runtime)
    return self
end

function Runtime:use_validator(validator: protocol.RequestValidator): Runtime
    table.insert(self.validators, validator)
    return self
end

function Runtime:register_handler(plugin_name: string, handler: protocol.PluginHandler): Runtime
    self.handlers[plugin_name] = handler
    return self
end

function Runtime:use_fallback(fallback: protocol.FallbackHandler): Runtime
    table.insert(self.fallbacks, fallback)
    return self
end

function Runtime:on_step(hook: protocol.StepHook): Runtime
    table.insert(self.hooks, hook)
    return self
end

function Runtime:new_store(id: string, now: time.Time): plugin_store.PluginStore
    return plugin_store.new(id, now)
end

function Runtime:emit(store: plugin_store.PluginStore, step: protocol.RuntimeStep, at: time.Time)
    store:push_step(step, at)
    for _, hook in ipairs(self.hooks) do
        hook(step, store.state)
    end
end

function Runtime:dispatch(
    store: plugin_store.PluginStore,
    request: protocol.Request,
    at: time.Time
): protocol.DispatchResult
    if request.kind == "tick" then
        store.state.last_tick = request.at
        local audit_step: protocol.AuditStep = {
            kind = "audit",
            note = "tick",
            at = request.at,
        }
        self:emit(store, audit_step, at)
        return {ok = true, value = nil}
    end

    for _, validator in ipairs(self.validators) do
        local validation = validator(store.state, request)
        if not validation.ok then
            return {ok = false, error = validation.error}
        end
    end

    local cached = store:lookup_cached(helpers.cache_key(request))
    if cached then
        local receipt: protocol.OutputReceipt = {
            plugin = cached.value.plugin,
            envelope_id = cached.value.envelope_id,
            output = cached.value.output,
            emitted_at = cached.value.emitted_at,
            cached = true,
        }
        local cached_step: protocol.CachedStep = {
            kind = "cached",
            plugin = request.plugin,
            envelope_id = request.envelope.id,
            at = at,
        }
        self:emit(store, cached_step, at)
        return {ok = true, value = receipt}
    end

    local handler = self.handlers[request.plugin]
    if not handler then
        return {
            ok = false,
            error = {
                code = "not_found",
                message = "missing handler: " .. request.plugin,
                retryable = false,
            },
        }
    end

    local handled = handler(store.state, request.envelope, at)
    if not handled.ok then
        for _, fallback in ipairs(self.fallbacks) do
            local fallback_result = fallback(store.state, request, handled.error, at)
            if not fallback_result.ok then
                return {ok = false, error = fallback_result.error}
            end

            local plan = fallback_result.value
            if plan then
                local fallback_step: protocol.FallbackStep = {
                    kind = "fallback",
                    plugin = request.plugin,
                    queue = plan.queue,
                    note = plan.note,
                    retry_at = plan.retry_at,
                }
                self:emit(store, fallback_step, at)
                return {ok = true, value = nil}
            end
        end

        return {ok = false, error = handled.error}
    end

    local receipt = handled.value
    if not receipt then
        return {ok = true, value = nil}
    end

    store:cache_receipt(request, receipt, at)

    local dispatch_step: protocol.DispatchStep = {
        kind = "dispatch",
        plugin = request.plugin,
        output = receipt.output,
        cached = receipt.cached,
    }
    self:emit(store, dispatch_step, at)
    return {ok = true, value = receipt}
end

function Runtime:run(
    store: plugin_store.PluginStore,
    requests: {protocol.Request},
    now: time.Time
): protocol.RunResult
    local last_output_kind: string? = nil

    for _, request in ipairs(requests) do
        local dispatch_result = self:dispatch(store, request, now)
        if not dispatch_result.ok then
            return {ok = false, error = dispatch_result.error}
        end

        local receipt = dispatch_result.value
        if receipt then
            last_output_kind = receipt.output.kind
        end
    end

    return {
        ok = true,
        value = store:summarize(now, last_output_kind),
    }
end

return M
