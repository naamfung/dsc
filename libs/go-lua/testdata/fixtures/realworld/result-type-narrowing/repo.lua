local result = require("result")

type User = {id: string, name: string, email: string, active: boolean}

local M = {}
M.User = User

local users: {[string]: User} = {
    ["u1"] = {id = "u1", name = "Alice", email = "alice@test.com", active = true},
    ["u2"] = {id = "u2", name = "Bob", email = "bob@test.com", active = false},
}

function M.find_by_id(id: string): Result<User>
    local user = users[id]
    if not user then
        return result.err("user not found: " .. id)
    end
    return result.ok(user)
end

function M.find_active(id: string): Result<User>
    local r = M.find_by_id(id)
    return result.and_then(r, function(user: User): Result<User>
        if not user.active then
            return result.err("user is inactive: " .. user.name)
        end
        return result.ok(user)
    end)
end

return M
