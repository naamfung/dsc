local function maybeCheck(val: any)
    if val == nil then
        print("warning: nil value")
    end
end
function process(x: string?)
    maybeCheck(x)
    local s: string = x -- expect-error
end
