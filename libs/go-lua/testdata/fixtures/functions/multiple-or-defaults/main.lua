local function format(value, prefix, suffix)
    local p = prefix or "["
    local s = suffix or "]"
    return p .. tostring(value) .. s
end
format(42)
format(42, "<")
format(42, "<", ">")
