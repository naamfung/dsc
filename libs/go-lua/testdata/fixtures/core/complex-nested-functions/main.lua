function outer(x: number): number
    local function inner(y: number): number
        return y * 2
    end
    return inner(x) + 1
end
