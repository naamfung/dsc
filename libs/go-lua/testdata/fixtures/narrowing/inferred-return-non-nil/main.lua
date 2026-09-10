local function getOrDefault(val: string?, default: string): string
    if val == nil then
        return default
    end
    return val
end
local s: string = getOrDefault(nil, "default")
