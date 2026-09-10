local function must(val: any, msg: string)
    assert(val, "must: " .. msg)
end
function process(x: number?)
    must(x, "x is required")
    local n: number = x
end
