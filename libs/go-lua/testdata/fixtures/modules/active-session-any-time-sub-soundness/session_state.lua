local time = require("time")

type ActiveSession = {
    created_at: any,
    last_activity: any,
}

local M = {}

function M.new(): ActiveSession
    local now = time.now()
    return {
        created_at = now,
        last_activity = now,
    }
end

return M
