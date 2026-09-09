type ErrorCode = "not_found" | "invalid" | "busy" | "rate_limited"

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

return M
