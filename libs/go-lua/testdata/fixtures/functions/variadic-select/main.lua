local function test(...: number)
    local count = select("#", ...)
    local first = select(1, ...)
end
test(1, 2, 3)
