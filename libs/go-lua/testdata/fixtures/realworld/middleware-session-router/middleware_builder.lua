local result = require("result")
local protocol = require("protocol")
local session_store = require("session_store")

type MiddlewareBuilder = {
    name: string,
    required_header: string?,
    local_tag_key: string?,
    local_field: string?,
    store: session_store.SessionStore?,
    required_scope: string?,
    named: (self: MiddlewareBuilder, name: string) -> MiddlewareBuilder,
    require_header: (self: MiddlewareBuilder, header: string) -> MiddlewareBuilder,
    copy_tag_to_local: (self: MiddlewareBuilder, tag_key: string, local_field: string) -> MiddlewareBuilder,
    load_sessions_from: (self: MiddlewareBuilder, store: session_store.SessionStore) -> MiddlewareBuilder,
    require_scope: (self: MiddlewareBuilder, scope: string) -> MiddlewareBuilder,
    build: (self: MiddlewareBuilder) -> protocol.Middleware,
}

type Builder = MiddlewareBuilder

local Builder = {}
Builder.__index = Builder

local M = {}

function M.new(): MiddlewareBuilder
    local self: Builder = {
        name = "middleware",
        required_header = nil,
        local_tag_key = nil,
        local_field = nil,
        store = nil,
        required_scope = nil,
        named = Builder.named,
        require_header = Builder.require_header,
        copy_tag_to_local = Builder.copy_tag_to_local,
        load_sessions_from = Builder.load_sessions_from,
        require_scope = Builder.require_scope,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:named(name: string): Builder
    self.name = name
    return self
end

function Builder:require_header(header: string): Builder
    self.required_header = header
    return self
end

function Builder:copy_tag_to_local(tag_key: string, local_field: string): Builder
    self.local_tag_key = tag_key
    self.local_field = local_field
    return self
end

function Builder:load_sessions_from(store: session_store.SessionStore): Builder
    self.store = store
    return self
end

function Builder:require_scope(scope: string): Builder
    self.required_scope = scope
    return self
end

function Builder:build(): protocol.Middleware
    local name = self.name
    local required_header = self.required_header
    local local_tag_key = self.local_tag_key
    local local_field = self.local_field
    local store = self.store
    local required_scope = self.required_scope

    return function(ctx: protocol.RequestContext): protocol.MiddlewareResult
        local next_ctx: protocol.RequestContext = ctx
        if required_header then
            local token = next_ctx.request.headers[required_header]
            if not token then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = name .. " missing header: " .. required_header,
                        retryable = false,
                    },
                }
            end

            if store then
                local snapshot = store:lookup(token)
                if not snapshot then
                    return {
                        ok = false,
                        error = {
                            code = "not_found",
                            message = name .. " missing session",
                            retryable = false,
                        },
                    }
                end
                if required_scope and not snapshot.scopes[required_scope] then
                    return {
                        ok = false,
                        error = {
                            code = "invalid",
                            message = name .. " denied",
                            retryable = false,
                        },
                    }
                end
                next_ctx.session = snapshot
            end
        end

        if local_tag_key and local_field then
            local tags = next_ctx.request.meta.tags
            if tags then
                local tag = tags[local_tag_key]
                if tag then
                    next_ctx.locals[local_field] = tag
                end
            end
        end

        return {ok = true, value = next_ctx}
    end
end

return M
