local function outer(f: (number) -> number)
    return f(10)
end
local result: number = outer(function(x: number): number
    return x * 2
end)
