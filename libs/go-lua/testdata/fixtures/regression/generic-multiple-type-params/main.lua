local function map<T, U>(arr: {T}, fn: (T) -> U): {U}
    local result: {U} = {}
    for i, v in ipairs(arr) do
        result[i] = fn(v)
    end
    return result
end
local nums = map({"a", "bb", "ccc"}, function(s: string): number
    return #s
end)
