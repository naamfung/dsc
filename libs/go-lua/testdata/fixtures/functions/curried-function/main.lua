local function createAdder(x: number): (number) -> number
    return function(y: number): number
        return x + y
    end
end
local add5 = createAdder(5)
local result: number = add5(3)
