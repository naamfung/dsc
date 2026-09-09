local function apply<T, U>(value: T, fn: fun(x: T): U): U
    return fn(value)
end
local result: string = apply(42, function(n: number): string
    return tostring(n)
end)
