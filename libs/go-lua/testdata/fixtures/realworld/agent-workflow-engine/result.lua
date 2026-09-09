type ErrorCode = "not_found" | "invalid" | "busy"

type AppError = {
    code: ErrorCode,
    message: string,
    retryable: boolean,
}

type Result<T> = {ok: true, value: T} | {ok: false, error: AppError}

local M = {}
M.ErrorCode = ErrorCode
M.AppError = AppError
M.Result = Result

function M.ok<T>(value: T): Result<T>
    return {ok = true, value = value}
end

function M.err<T>(code: ErrorCode, message: string, retryable: boolean?): Result<T>
    return {
        ok = false,
        error = {
            code = code,
            message = message,
            retryable = retryable or false,
        },
    }
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
