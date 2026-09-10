local validate = require("validate")

type ProcessResult = {
    message: string,
    config: validate.ValidConfig,
}

local M = {}

function M.run(input: string): (ProcessResult?, string?)
    local config, err = validate.parse_and_validate(input)
    if err then
        return nil, "process: " .. err
    end
    if not config then
        return nil, "config is nil"
    end
    local result: ProcessResult = {
        message = "Configured " .. config.host .. ":" .. tostring(config.port),
        config = config,
    }
    return result, nil
end

return M
