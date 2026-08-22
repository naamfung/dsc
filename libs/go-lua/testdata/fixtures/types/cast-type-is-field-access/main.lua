type Point = {x: number, y: number}
local v: any = {x = 1, y = 2}
local p, err = Point:is(v)
if err == nil then
    local sum = p.x + p.y
end
