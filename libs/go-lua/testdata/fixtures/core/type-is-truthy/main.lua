type Point = {x: number, y: number}
local function isPoint(x)
    return Point:is(x)
end
function validate(data: any)
    local val, err = isPoint(data)
    if err == nil and val ~= nil then
        local p: {x: number, y: number} = val
    end
end
