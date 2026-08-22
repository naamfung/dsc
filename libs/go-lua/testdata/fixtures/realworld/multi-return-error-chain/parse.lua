type ParsedConfig = {
    host: string,
    port: number,
    debug: boolean,
}

local M = {}
M.ParsedConfig = ParsedConfig

function M.parse_string(input: string): (ParsedConfig?, string?)
    if #input == 0 then
        return nil, "empty input"
    end
    return {host = "localhost", port = 8080, debug = false}, nil
end

function M.parse_number(input: string): (number?, string?)
    local n = tonumber(input)
    if not n then
        return nil, "invalid number: " .. input
    end
    return n, nil
end

return M
