local status = require("status")

type Route = {
    method: status.HttpMethod,
    path: string,
    handler: (req: status.Request) -> status.Response,
}

type Router = {
    _routes: {Route},
    add: (self: Router, method: status.HttpMethod, path: string, handler: (req: status.Request) -> status.Response) -> Router,
    handle: (self: Router, req: status.Request) -> status.Response,
}

local M = {}

function M.new(): Router
    local router: Router = {
        _routes = {},
        add = function(self: Router, method: status.HttpMethod, path: string, handler: (req: status.Request) -> status.Response): Router
            table.insert(self._routes, {method = method, path = path, handler = handler})
            return self
        end,
        handle = function(self: Router, req: status.Request): status.Response
            for _, route in ipairs(self._routes) do
                if route.method == req.method and route.path == req.path then
                    return route.handler(req)
                end
            end
            return status.error(404, "Not found: " .. req.method .. " " .. req.path)
        end,
    }
    return router
end

return M
