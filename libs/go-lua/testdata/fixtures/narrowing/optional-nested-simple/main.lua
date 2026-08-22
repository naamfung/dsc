type Error = {kind: string, message: string}
local function test(): Error?
    return {kind = "test", message = "msg"}
end
local err = test()
if err then
    local msg = err.message
end
