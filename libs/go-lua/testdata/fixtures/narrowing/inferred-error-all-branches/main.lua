local function fail(msg: string)
    error(msg)
end
function process(x: string?): string
    if x == nil then
        fail("x is nil")
    end
    return x
end
