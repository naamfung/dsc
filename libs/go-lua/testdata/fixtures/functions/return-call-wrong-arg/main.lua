local function add(a: number, b: number): number
    return a + b
end
local function f(): number
    return add("bad", 2) -- expect-error
end
local x = f()
