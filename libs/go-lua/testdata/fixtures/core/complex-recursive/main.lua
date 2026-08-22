function factorial(n: number): number
    if n <= 1 then
        return 1
    else
        return n * factorial(n - 1)
    end
end
