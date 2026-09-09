local function assertNotNil(val: any)
    if val == nil then
        error("value is nil")
    end
end
function process(x: string?)
    assertNotNil(x)
    local s: string = x
end
