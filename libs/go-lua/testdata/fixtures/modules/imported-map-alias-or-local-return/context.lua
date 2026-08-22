type Context = {[string]: any}

local M = {}
M.Context = Context

function M.empty(): Context
    return {}
end

return M
