local function inner(): (string?, string?)
    return nil, "error"
end
local function outer(): (string?, string?)
    local result, err = inner()
    if err ~= nil then
        return nil, err
    end
    return result, nil
end
