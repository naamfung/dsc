type Request = { query: (self: Request, key: string) -> (string?, Error?) }
local function handler(req: Request)
    local code = tonumber(req:query("code")) or 200
    return code
end
