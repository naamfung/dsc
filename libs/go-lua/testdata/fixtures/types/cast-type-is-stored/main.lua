type Point = {x: number, y: number}
local function validate(data: any)
    local val, err = Point:is(data)
    if err == nil then
        local sum = val.x + val.y
    end
end
