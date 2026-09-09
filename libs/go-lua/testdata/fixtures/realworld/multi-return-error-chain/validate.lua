local parse = require("parse")

type ValidConfig = {
    host: string,
    port: number,
    debug: boolean,
    validated: true,
}

local M = {}
M.ValidConfig = ValidConfig

function M.validate(config: parse.ParsedConfig): (ValidConfig?, string?)
    if #config.host == 0 then
        return nil, "host is empty"
    end
    if config.port < 1 or config.port > 65535 then
        return nil, "port out of range: " .. tostring(config.port)
    end
    local valid: ValidConfig = {
        host = config.host,
        port = config.port,
        debug = config.debug,
        validated = true,
    }
    return valid, nil
end

function M.parse_and_validate(input: string): (ValidConfig?, string?)
    local parsed, parse_err = parse.parse_string(input)
    if parse_err then
        return nil, "parse: " .. parse_err
    end
    if not parsed then
        return nil, "parse returned nil"
    end
    return M.validate(parsed)
end

return M
