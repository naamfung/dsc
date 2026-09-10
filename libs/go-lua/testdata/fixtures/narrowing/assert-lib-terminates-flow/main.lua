local assert = {
    not_nil = function(val: any, msg: string?)
        if val == nil then error(msg or "nil") end
    end
}
function getOrFail(x: string?): string
    assert.not_nil(x, "x must not be nil")
    return x
end
