local x: string | number = "hello"
if type(x) == "string" then
    local s: string = x
end

local y: string | number = 42
if type(y) == "string" then
    local n: number = y -- expect-error: cannot assign
end
