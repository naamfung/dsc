local function outer(a: number): (number) -> (number) -> number
    return function(b: number): (number) -> number
        return function(c: number): number
            return a + b + c
        end
    end
end
local result: number = outer(1)(2)(3)
