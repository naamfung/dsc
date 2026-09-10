local function add(a: number, b: number): number
    return a + b
end
local x: string = add(1, 2) -- expect-error
