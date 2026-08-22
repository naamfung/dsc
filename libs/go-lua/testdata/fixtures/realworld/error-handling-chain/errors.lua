type AppError = {
    code: string,
    message: string,
    retryable: boolean
}

local M = {}
M.AppError = AppError

function M.new(code: string, message: string, retryable: boolean?): AppError
    return {
        code = code,
        message = message,
        retryable = retryable or false
    }
end

function M.is_retryable(err: AppError): boolean
    return err.retryable
end

function M.wrap(err: AppError, context: string): AppError
    return M.new(err.code, context .. ": " .. err.message, err.retryable)
end

return M
