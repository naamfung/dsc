local function sum(...: number): number
    return 0
end
local x = sum(1, 2, "three") -- expect-error
