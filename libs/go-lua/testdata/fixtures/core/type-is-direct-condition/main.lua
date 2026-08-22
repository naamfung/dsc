type Point = {x: number, y: number}
function validate(data: any)
    local _, err = Point:is(data)
    if err == nil then
        local p: {x: number, y: number} = data
    end
end
