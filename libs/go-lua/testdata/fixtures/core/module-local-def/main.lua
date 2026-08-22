local M = {}
function M.add(a: number, b: number): number
    return a + b
end
function M.sub(a: number, b: number): number
    return a - b
end
local result: number = M.add(1, 2)
