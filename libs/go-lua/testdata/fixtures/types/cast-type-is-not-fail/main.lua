type Point = {x: number, y: number}
local function validate(data: any)
    if not Point:is(data) then
        local p: {x: number, y: number} = data -- expect-error
    end
end
