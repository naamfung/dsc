type Error = {kind: (self: Error) -> string, message: (self: Error) -> string, retryable: (self: Error) -> boolean}
local function test(): Error?
    return nil
end
local err = test()
local a, b, c = true, true, true
if err then
    if a then
        if b then
            if c then
                local k = err:kind()
                local m = err:message()
                local r = err:retryable()
            end
        end
    end
end
