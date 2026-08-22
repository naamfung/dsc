type Config = {name: string, port?: number}
local function get_port(c: Config): number
    if c.port ~= nil then
        return c.port
    end
    return 8080
end
