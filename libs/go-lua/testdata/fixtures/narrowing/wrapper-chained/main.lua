local function assertNotNil(val: any)
    assert(val, "not nil")
end
function process(a: string?, b: number?)
    assertNotNil(a)
    assertNotNil(b)
    local s: string = a
    local n: number = b
end
