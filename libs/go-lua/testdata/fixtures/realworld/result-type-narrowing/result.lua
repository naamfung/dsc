type Result<T> = {ok: true, value: T} | {ok: false, error: string}

local M = {}
M.Result = Result

function M.ok<T>(value: T): Result<T>
    return {ok = true, value = value}
end

function M.err<T>(message: string): Result<T>
    return {ok = false, error = message}
end

function M.map<T, U>(r: Result<T>, fn: (T) -> U): Result<U>
    if r.ok then
        return M.ok(fn(r.value))
    end
    return {ok = false, error = r.error}
end

function M.and_then<T, U>(r: Result<T>, fn: (T) -> Result<U>): Result<U>
    if r.ok then
        return fn(r.value)
    end
    return {ok = false, error = r.error}
end

return M
