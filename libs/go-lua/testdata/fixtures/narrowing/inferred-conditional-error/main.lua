local function maybeError(cond: boolean)
    if cond then
        error("condition was true")
    end
end
function process(x: string?)
    maybeError(x == nil)
    local s: string = x
end
