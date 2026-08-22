local function pair<A, B>(a: A, b: B): (A, B)
    return a, b
end
local n, s = pair(42, "hello")
local x: number = n
local y: string = s
