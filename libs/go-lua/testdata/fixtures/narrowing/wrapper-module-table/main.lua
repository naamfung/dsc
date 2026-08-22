local check = {}
function check.notNil(val: any, msg: string?)
    assert(val, msg or "value is nil")
end
function process(x: string?)
    check.notNil(x)
    local s: string = x
end
