local M = {}

function M.eq(actual, expected, msg)
    if actual ~= expected then
        error(msg or "assertion failed", 2)
    end
end

function M.not_nil(val, msg)
    if val == nil then
        error(msg or "expected non-nil", 2)
    end
    return val
end

return M
