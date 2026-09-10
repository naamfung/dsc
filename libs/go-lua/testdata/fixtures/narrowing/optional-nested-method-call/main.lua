type Error = {kind: (self: Error) -> string, message: (self: Error) -> string}
local function test(): Error?
    return nil
end
local err = test()
if err then
    local msg = err:message()
end
