local time = require("time")
local result = require("result")

type RequestMeta = {
    trace_id: string,
    tags: {[string]: string}?,
}

type HttpRequest = {
    kind: "http",
    method: "GET" | "POST",
    path: string,
    headers: {[string]: string},
    params: {[string]: string}?,
    body: string?,
    meta: RequestMeta,
}

type TimerRequest = {
    kind: "timer",
    id: string,
    at: time.Time,
    meta: RequestMeta,
}

type Request = HttpRequest | TimerRequest

type SessionSnapshot = {
    id: string,
    user_id: string,
    scopes: {[string]: boolean},
    last_seen: time.Time?,
    attributes: {[string]: string}?,
}

type RequestContext = {
    request: HttpRequest,
    params: {[string]: string},
    locals: {[string]: string},
    session: SessionSnapshot?,
}

type Response = {
    status: integer,
    body: string,
    headers: {[string]: string},
}

type AppError = result.AppError
type ResponseResult = {ok: true, value: Response} | {ok: false, error: AppError}
type MiddlewareResult = {ok: true, value: RequestContext} | {ok: false, error: AppError}
type Middleware = (RequestContext) -> MiddlewareResult
type RouteHandler = (RequestContext) -> ResponseResult
type AfterHook = (RequestContext, Response) -> ()

type Route = {
    key: string,
    middlewares: {Middleware},
    handle: RouteHandler,
}

local M = {}
M.RequestMeta = RequestMeta
M.HttpRequest = HttpRequest
M.TimerRequest = TimerRequest
M.Request = Request
M.SessionSnapshot = SessionSnapshot
M.RequestContext = RequestContext
M.Response = Response
M.ResponseResult = ResponseResult
M.MiddlewareResult = MiddlewareResult
M.Middleware = Middleware
M.RouteHandler = RouteHandler
M.AfterHook = AfterHook
M.Route = Route

function M.meta(trace_id: string, tags: {[string]: string}?): RequestMeta
    return {
        trace_id = trace_id,
        tags = tags,
    }
end

return M
