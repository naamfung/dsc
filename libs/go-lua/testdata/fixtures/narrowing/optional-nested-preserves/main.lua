type Error = {kind: string, message: string}
local function test(): Error?
    return {kind = "test", message = "msg"}
end
local err = test()
local flag = true
if err then
    if flag then
        local msg = err.message
    end
end
