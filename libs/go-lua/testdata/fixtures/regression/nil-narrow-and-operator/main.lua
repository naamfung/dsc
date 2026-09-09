local function safe_concat(a: string?, b: string): string
    if a ~= nil then
        return a .. b
    end
    return b
end
