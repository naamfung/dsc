local function assertNotNil(val: any)
    assert(val, "value must not be nil")
end
function process(obj: {data: string?})
    assertNotNil(obj.data)
    local s: string = obj.data
end
