type Value = number
local a: Value = 10
if true then
    type Value = string
    local b: Value = "hello"
end
local c: Value = 20
