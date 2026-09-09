local tests = {}

function tests.is_nil(val: any, msg: string?)
    if val ~= nil then
        error(msg or "expected nil", 2)
    end
end

function tests.eq(actual: any, expected: any, msg: string?)
    if actual ~= expected then
        error(msg or "not equal", 2)
    end
end

return tests
