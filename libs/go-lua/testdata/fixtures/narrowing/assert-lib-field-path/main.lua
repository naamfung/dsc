local assert = {
    not_nil = function(val: any, msg: string?)
        if val == nil then error(msg or "nil") end
    end
}
function process(obj: {stream: {read: () -> string}?})
    assert.not_nil(obj.stream)
    local s: string = obj.stream:read()
end
