local function add(a: number, b: number): number
    return a + b
end

local function mul(a: number, b: number): number
    return a * b
end

print(add(10, 20))
print(mul(3, 4))
print(add(1, mul(2, 3)))
