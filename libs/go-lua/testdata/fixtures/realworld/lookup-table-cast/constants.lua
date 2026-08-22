type FinishReason = "stop" | "length" | "tool_call" | "content_filter"
type ErrorType = "invalid_request" | "authentication" | "rate_limit" | "server_error" | "model_error"

local M = {}

M.FINISH_REASON = {
    STOP = "stop",
    LENGTH = "length",
    TOOL_CALL = "tool_call",
    CONTENT_FILTER = "content_filter",
}

M.ERROR_TYPE = {
    INVALID_REQUEST = "invalid_request",
    AUTHENTICATION = "authentication",
    RATE_LIMIT = "rate_limit",
    SERVER_ERROR = "server_error",
    MODEL_ERROR = "model_error",
}

return M
