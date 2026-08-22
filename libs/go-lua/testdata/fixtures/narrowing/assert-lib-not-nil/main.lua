local assert = {
    not_nil = function(val: any, msg: string?)
        if val == nil then error(msg or "assertion failed") end
    end
}
function process(x: string?)
    assert.not_nil(x)
    local s: string = x
end
