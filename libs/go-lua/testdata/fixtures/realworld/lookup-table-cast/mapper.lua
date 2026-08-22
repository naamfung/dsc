local constants = require("constants")

type FinishReasonMap = {[string]: string}
type ErrorTypeMap = {[string]: string}
type StatusCodeMap = {[number]: string}

local M = {}

local finish_reasons: FinishReasonMap = {}
finish_reasons["end_turn"] = constants.FINISH_REASON.STOP
finish_reasons["max_tokens"] = constants.FINISH_REASON.LENGTH
finish_reasons["stop_sequence"] = constants.FINISH_REASON.STOP
finish_reasons["tool_use"] = constants.FINISH_REASON.TOOL_CALL
M.finish_reasons = finish_reasons

local error_types: ErrorTypeMap = {}
error_types["invalid_request_error"] = constants.ERROR_TYPE.INVALID_REQUEST
error_types["authentication_error"] = constants.ERROR_TYPE.AUTHENTICATION
error_types["rate_limit_error"] = constants.ERROR_TYPE.RATE_LIMIT
error_types["api_error"] = constants.ERROR_TYPE.SERVER_ERROR
M.error_types = error_types

local status_codes: StatusCodeMap = {}
status_codes[400] = constants.ERROR_TYPE.INVALID_REQUEST
status_codes[401] = constants.ERROR_TYPE.AUTHENTICATION
status_codes[429] = constants.ERROR_TYPE.RATE_LIMIT
status_codes[500] = constants.ERROR_TYPE.SERVER_ERROR
M.status_codes = status_codes

function M.map_finish_reason(api_reason: string): string
    return M.finish_reasons[api_reason] or "unknown"
end

function M.map_error_type(api_error: string): string
    return M.error_types[api_error] or constants.ERROR_TYPE.SERVER_ERROR
end

function M.map_status_code(code: number): string
    return M.status_codes[code] or constants.ERROR_TYPE.SERVER_ERROR
end

return M
