type Point = {x: number, y: number}
local function validate(data: any)
    local val = Point:is(data)
    if not val then
        local p: {x: number, y: number} = data -- expect-error
    end
end
