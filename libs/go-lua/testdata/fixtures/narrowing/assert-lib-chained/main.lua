local assert = {
    not_nil = function(val: any, msg: string?)
        if val == nil then error(msg or "nil") end
    end
}
function process(a: string?, b: number?)
    assert.not_nil(a, "a")
    assert.not_nil(b, "b")
    local s: string = a
    local n: number = b
end
