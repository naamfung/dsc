local function fib(n: number): number
    if n < 2 then return n end
    return fib(n - 1) + fib(n - 2)
end

print(fib(10))
