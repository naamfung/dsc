local function assertEq(a: any, b: any)
    if a ~= b then error("not equal") end
end
function process(x: string | number, y: string)
    assertEq(x, y)
    local s: string = x
end
