local protocol = require("protocol")
local result = require("result")

type NoteDecorator = (string, protocol.DispatchRequest, result.AppError) -> string

type FallbackBuilder = {
    plugin_name: string?,
    retry_code: result.ErrorCode?,
    queue_name: string?,
    decorator: NoteDecorator?,
    for_plugin: (self: FallbackBuilder, plugin_name: string) -> FallbackBuilder,
    retry_on: (self: FallbackBuilder, code: result.ErrorCode) -> FallbackBuilder,
    queue_named: (self: FallbackBuilder, queue_name: string) -> FallbackBuilder,
    decorate_note: (self: FallbackBuilder, fn: NoteDecorator) -> FallbackBuilder,
    build: (self: FallbackBuilder) -> protocol.FallbackHandler,
}

type Builder = FallbackBuilder

local Builder = {}
Builder.__index = Builder

local M = {}
M.FallbackBuilder = FallbackBuilder

function M.new(): FallbackBuilder
    local self: Builder = {
        plugin_name = nil,
        retry_code = nil,
        queue_name = nil,
        decorator = nil,
        for_plugin = Builder.for_plugin,
        retry_on = Builder.retry_on,
        queue_named = Builder.queue_named,
        decorate_note = Builder.decorate_note,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:for_plugin(plugin_name: string): Builder
    self.plugin_name = plugin_name
    return self
end

function Builder:retry_on(code: result.ErrorCode): Builder
    self.retry_code = code
    return self
end

function Builder:queue_named(queue_name: string): Builder
    self.queue_name = queue_name
    return self
end

function Builder:decorate_note(fn: NoteDecorator): Builder
    self.decorator = fn
    return self
end

function Builder:build(): protocol.FallbackHandler
    local plugin_name = self.plugin_name
    local retry_code = self.retry_code
    local queue_name = self.queue_name or "retry"
    local decorator = self.decorator

    return function(
        state: protocol.StoreState,
        request: protocol.DispatchRequest,
        err: result.AppError,
        at: time.Time
    ): protocol.FallbackResult
        if plugin_name and request.plugin ~= plugin_name then
            return {ok = true, value = nil}
        end

        if retry_code and err.code ~= retry_code then
            return {ok = true, value = nil}
        end

        state.flags["saw_fallback"] = true

        local note = queue_name .. ":" .. request.plugin .. ":" .. request.envelope.id
        if decorator then
            note = decorator(note, request, err)
        end

        return {
            ok = true,
            value = {
                queue = queue_name,
                note = note,
                retry_at = at,
            },
        }
    end
end

return M
