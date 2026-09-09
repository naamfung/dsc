local function div(a: number, b: number): (number?, string?)
    if b == 0 then
        return nil, "division by zero"
    end
    return a / b, nil
end
local result, err = div(10, 2)
