local result = require("result")
local repo = require("repo")

type Greeting = {message: string, user_name: string}

local M = {}

function M.greet_user(id: string): Result<Greeting>
    local user_result = repo.find_active(id)

    return result.map(user_result, function(user: User): Greeting
        return {
            message = "Hello, " .. user.name .. "!",
            user_name = user.name,
        }
    end)
end

function M.get_email(id: string): (string?, string?)
    local r = repo.find_by_id(id)
    if r.ok then
        return r.value.email, nil
    end
    return nil, r.error
end

return M
