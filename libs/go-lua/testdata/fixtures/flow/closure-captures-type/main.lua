local function make_adder(n: number): fun(x: number): number
    return function(x: number): number
        return x + n
    end
end
local add5 = make_adder(5)
local result: number = add5(10)
