type Point = {x: number, y: number}
local function validate(data: any)
    local val, err = Point:is(data)
    if err == nil then
        local p: {x: number, y: number} = val
        local sum = p.x + p.y
    end
end
