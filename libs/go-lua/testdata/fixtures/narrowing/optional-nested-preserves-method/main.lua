type Error = {kind: (self: Error) -> string, message: (self: Error) -> string}
local function test(): Error?
    return nil
end
local err = test()
local flag = true
if err then
    if flag then
        local msg = err:message()
    end
end
