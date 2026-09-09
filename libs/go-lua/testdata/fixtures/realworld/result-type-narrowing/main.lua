local service = require("service")

local greeting = service.greet_user("u1")
if greeting.ok then
    local msg: string = greeting.value.message
    local name: string = greeting.value.user_name
end

local fail = service.greet_user("u2")
if not fail.ok then
    local err_msg: string = fail.error
end

local email, err = service.get_email("u1")
if err == nil then
    local e: string = email
end
