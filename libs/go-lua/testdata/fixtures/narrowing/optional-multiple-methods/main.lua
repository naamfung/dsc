type Error = {kind: (self: Error) -> string, message: (self: Error) -> string, retryable: (self: Error) -> boolean}
local function test(): Error?
    return nil
end
local err = test()
if err then
    local kind = err:kind()
    if kind == "network" then
        local retryable = err:retryable()
        local message = err:message()
    end
end
