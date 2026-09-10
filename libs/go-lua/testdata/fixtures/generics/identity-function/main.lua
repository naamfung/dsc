local function identity<T>(x: T): T
    return x
end
local n: number = identity(42)
local s: string = identity("hello")
