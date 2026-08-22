function f(x: string?): string
    if x == nil then
        error("x is nil")
    end
    return x
end
