local errors = require("errors")
local validator = require("validator")

-- Use exported type for runtime validation
local raw_data = {code = "TEST", message = "hello", retryable = false}
local validated, type_err = errors.AppError:is(raw_data)
if type_err == nil and validated then
    local code: string = validated.code
    local msg: string = validated.message
end

-- Normal flow with typed functions
local result = validator.validate_name("Alice")
if result.ok then
    local name: string = result.value
else
    local err = result.error
    local wrapped = errors.wrap(err, "registration")
    local code: string = wrapped.code
    local msg: string = wrapped.message
    local retry: boolean = errors.is_retryable(wrapped)
end

-- Validate empty input produces error
local fail_result = validator.validate_name("")
if not fail_result.ok then
    local err_code: string = fail_result.error.code
end
