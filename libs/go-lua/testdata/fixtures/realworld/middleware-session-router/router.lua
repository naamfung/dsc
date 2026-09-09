local time = require("time")
local result = require("result")
local protocol = require("protocol")

type Router = {
    routes: {[string]: protocol.Route},
    global_middlewares: {protocol.Middleware},
    hooks: {protocol.AfterHook},
    register_route: (self: Router, route: protocol.Route) -> Router,
    use: (self: Router, middleware: protocol.Middleware) -> Router,
    on_response: (self: Router, hook: protocol.AfterHook) -> Router,
    dispatch: (self: Router, request: protocol.Request, now: time.Time) -> protocol.ResponseResult,
}

type RuntimeRouter = Router

local RuntimeRouter = {}
RuntimeRouter.__index = RuntimeRouter

local M = {}

function M.new(): Router
    local self: RuntimeRouter = {
        routes = {},
        global_middlewares = {},
        hooks = {},
        register_route = RuntimeRouter.register_route,
        use = RuntimeRouter.use,
        on_response = RuntimeRouter.on_response,
        dispatch = RuntimeRouter.dispatch,
    }
    setmetatable(self, RuntimeRouter)
    return self
end

function RuntimeRouter:register_route(route: protocol.Route): RuntimeRouter
    self.routes[route.key] = route
    return self
end

function RuntimeRouter:use(middleware: protocol.Middleware): RuntimeRouter
    table.insert(self.global_middlewares, middleware)
    return self
end

function RuntimeRouter:on_response(hook: protocol.AfterHook): RuntimeRouter
    table.insert(self.hooks, hook)
    return self
end

function RuntimeRouter:dispatch(request: protocol.Request, now: time.Time): protocol.ResponseResult
    if request.kind == "timer" then
        return {
            ok = true,
            value = {
                status = 202,
                body = "timer:" .. request.id .. ":" .. tostring(request.at:unix()),
                headers = {["x-trace"] = request.meta.trace_id},
            },
        }
    end

    local route = self.routes[request.method .. " " .. request.path]
    if not route then
        return {
            ok = false,
            error = {
                code = "not_found",
                message = "missing route: " .. request.path,
                retryable = false,
            },
        }
    end
    local route_value: protocol.Route = route

    local ctx: protocol.RequestContext = {
        request = request,
        params = request.params or {},
        locals = {},
        session = nil,
    }

    local current_ctx: protocol.RequestContext = ctx
    for _, middleware in ipairs(self.global_middlewares) do
        local step = middleware(current_ctx)
        if not step.ok then
            return {ok = false, error = step.error}
        end
        current_ctx = step.value
    end
    for _, middleware in ipairs(route_value.middlewares) do
        local step = middleware(current_ctx)
        if not step.ok then
            return {ok = false, error = step.error}
        end
        current_ctx = step.value
    end

    local final_ctx = current_ctx
    local response_result = route_value.handle(final_ctx)
    if response_result.ok then
        final_ctx.locals["handled_at"] = tostring(now:unix())
        for _, hook in ipairs(self.hooks) do
            hook(final_ctx, response_result.value)
        end
    end
    return response_result
end

return M
