local function innerAssert(val: any, msg: string)
    assert(val, msg)
end
local function outerAssert(val: any)
    innerAssert(val, "outer: value is nil")
end
function process(x: string?)
    outerAssert(x)
    local s: string = x
end
