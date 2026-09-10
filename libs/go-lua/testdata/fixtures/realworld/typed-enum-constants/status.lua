type HttpMethod = "GET" | "POST" | "PUT" | "DELETE" | "PATCH"
type StatusCode = 200 | 201 | 400 | 401 | 403 | 404 | 500

type Request = {
    method: HttpMethod,
    path: string,
    body: any?,
    headers: {[string]: string},
}

type Response = {
    status: StatusCode,
    body: any?,
    headers: {[string]: string},
}

local M = {}
M.HttpMethod = HttpMethod
M.Request = Request
M.Response = Response
M.StatusCode = StatusCode

M.METHOD = {
    GET = "GET",
    POST = "POST",
    PUT = "PUT",
    DELETE = "DELETE",
    PATCH = "PATCH",
}

M.STATUS = {
    OK = 200,
    CREATED = 201,
    BAD_REQUEST = 400,
    UNAUTHORIZED = 401,
    FORBIDDEN = 403,
    NOT_FOUND = 404,
    SERVER_ERROR = 500,
}

function M.ok(body: any?): Response
    return {status = 200, body = body, headers = {}}
end

function M.created(body: any?): Response
    return {status = 201, body = body, headers = {}}
end

function M.error(status: StatusCode, message: string): Response
    return {status = status, body = {error = message}, headers = {}}
end

return M
