local M = {}

function M.is_nil(val: any, msg: string?)
    if val ~= nil then
        error(msg or "expected nil", 2)
    end
end

return M
