local function conditionalAssert(val: any, check: boolean)
    if check then
        assert(val, "value is nil")
    end
end
function process(x: string?)
    conditionalAssert(x, true)
    local s: string = x -- expect-error
end
