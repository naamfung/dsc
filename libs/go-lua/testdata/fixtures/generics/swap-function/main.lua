local function swap<A, B>(a: A, b: B): (B, A)
    return b, a
end
local s, n = swap(42, "hello")
local x: string = s
local y: number = n
