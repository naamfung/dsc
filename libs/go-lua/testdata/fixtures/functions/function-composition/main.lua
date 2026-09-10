local function compose(f: (number) -> number, g: (number) -> number): (number) -> number
    return function(x: number): number
        return f(g(x))
    end
end
local function double(x: number): number return x * 2 end
local function addOne(x: number): number return x + 1 end
local composed = compose(double, addOne)
local result: number = composed(5)
