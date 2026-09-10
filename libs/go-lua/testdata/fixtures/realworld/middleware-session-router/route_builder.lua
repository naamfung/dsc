local result = require("result")
local protocol = require("protocol")

type BodyDecorator = (string, protocol.RequestContext) -> string

type RouteBuilder = {
    route_key: string,
    middlewares: {protocol.Middleware},
    required_param: string?,
    decorator: BodyDecorator?,
    handler: protocol.RouteHandler,
    key: (self: RouteBuilder, route_key: string) -> RouteBuilder,
    use: (self: RouteBuilder, middleware: protocol.Middleware) -> RouteBuilder,
    require_param: (self: RouteBuilder, param: string) -> RouteBuilder,
    decorate_body: (self: RouteBuilder, decorator: BodyDecorator) -> RouteBuilder,
    handle: (self: RouteBuilder, handler: protocol.RouteHandler) -> RouteBuilder,
    build: (self: RouteBuilder) -> protocol.Route,
}

type Builder = RouteBuilder

local Builder = {}
Builder.__index = Builder

local M = {}

local function missing_handler(_ctx: protocol.RequestContext): protocol.ResponseResult
    return {
        ok = false,
        error = {
            code = "invalid",
            message = "missing handler",
            retryable = false,
        },
    }
end

function M.new(): RouteBuilder
    local self: Builder = {
        route_key = "GET /",
        middlewares = {},
        required_param = nil,
        decorator = nil,
        handler = missing_handler,
        key = Builder.key,
        use = Builder.use,
        require_param = Builder.require_param,
        decorate_body = Builder.decorate_body,
        handle = Builder.handle,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:key(route_key: string): Builder
    self.route_key = route_key
    return self
end

function Builder:use(middleware: protocol.Middleware): Builder
    table.insert(self.middlewares, middleware)
    return self
end

function Builder:require_param(param: string): Builder
    self.required_param = param
    return self
end

function Builder:decorate_body(decorator: BodyDecorator): Builder
    self.decorator = decorator
    return self
end

function Builder:handle(handler: protocol.RouteHandler): Builder
    self.handler = handler
    return self
end

function Builder:build(): protocol.Route
    local route_key = self.route_key
    local middlewares = self.middlewares
    local required_param = self.required_param
    local decorator = self.decorator
    local handler = self.handler

    return {
        key = route_key,
        middlewares = middlewares,
        handle = function(ctx: protocol.RequestContext): protocol.ResponseResult
            if required_param then
                local raw = ctx.params[required_param]
                if not raw then
                    return {
                        ok = false,
                        error = {
                            code = "invalid",
                            message = "missing param: " .. required_param,
                            retryable = false,
                        },
                    }
                end
            end

            local response_result = handler(ctx)
            if not response_result.ok then
                return response_result
            end

            if decorator then
                local response = response_result.value
                return {
                    ok = true,
                    value = {
                        status = response.status,
                        body = decorator(response.body, ctx),
                        headers = response.headers,
                    },
                }
            end

            return response_result
        end,
    }
end

return M
