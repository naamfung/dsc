local function add(a: number, b: number): number
    return a + b
end
local x = add(1, "wrong") -- expect-error
