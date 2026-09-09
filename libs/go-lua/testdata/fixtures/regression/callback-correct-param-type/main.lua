type User = {name: string, age: number}
local function process_user(u: User, callback: (User) -> nil)
    callback(u)
end
process_user({name = "Alice", age = 30}, function(u: User)
    local n: string = u.name
end)
