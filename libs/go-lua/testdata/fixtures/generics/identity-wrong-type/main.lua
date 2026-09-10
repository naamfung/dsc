local function identity<T>(x: T): T
    return x
end
local n: number = identity("hello") -- expect-error
