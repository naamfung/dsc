local function require_value(x: string?): string
    if x == nil then
        return "missing"
    end
    return x
end
