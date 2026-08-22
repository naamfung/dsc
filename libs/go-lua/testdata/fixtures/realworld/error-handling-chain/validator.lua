local errors = require("errors")

type ValidationResult = {ok: true, value: string} | {ok: false, error: errors.AppError}

local M = {}
M.ValidationResult = ValidationResult

function M.validate_name(input: string): ValidationResult
    if #input == 0 then
        return {ok = false, error = errors.new("EMPTY", "name is empty")}
    end
    if #input > 100 then
        return {ok = false, error = errors.new("TOO_LONG", "name exceeds 100 chars")}
    end
    return {ok = true, value = input}
end

return M
