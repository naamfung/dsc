local function assertNotNil(val: any)
    assert(val, "value must not be nil")
end
function process(x: string?)
    assertNotNil(x)
    local s: string = x
end
