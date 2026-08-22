local M = {}

function M.eq(actual, expected, msg)
    if actual ~= expected then
        error(msg or "assertion failed", 2)
    end
end

return M
