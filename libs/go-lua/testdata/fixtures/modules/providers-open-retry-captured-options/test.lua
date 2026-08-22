local M = {}

function M.is_nil(val, msg)
    if val ~= nil then
        error(msg or "expected nil", 2)
    end
end

function M.not_nil(val, msg)
    if val == nil then
        error(msg or "expected non-nil", 2)
    end
    return val
end

return M
