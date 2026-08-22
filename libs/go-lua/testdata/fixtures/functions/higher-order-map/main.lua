local function map(arr: {number}, fn: (number) -> number): {number}
    local result: {number} = {}
    for i, v in ipairs(arr) do
        result[i] = fn(v)
    end
    return result
end
local doubled = map({1, 2, 3}, function(x: number): number return x * 2 end)
