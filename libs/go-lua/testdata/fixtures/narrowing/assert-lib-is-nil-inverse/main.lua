local assert = {
    is_nil = function(val: any, msg: string?)
        if val ~= nil then error(msg or "expected nil") end
    end
}
function process(x: string?, err: string?)
    assert.is_nil(err)
    local s: string = x -- expect-error
end
