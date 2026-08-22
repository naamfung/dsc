local function sum(...: number): number
    local result = 0
    for _, v in ipairs({...}) do
        result = result + v
    end
    return result
end
local total: number = sum(1, 2, 3)
