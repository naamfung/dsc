local function process(x: number): (number?, string?)
    if x < 0 then
        return nil, "negative"
    end
    return x * 2, nil
end
local result, err = process(5)
if err ~= nil then
    return
end
local n: number = result
