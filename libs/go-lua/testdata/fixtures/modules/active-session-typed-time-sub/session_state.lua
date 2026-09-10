local time = require("time")

type ActiveSession = {
    created_at: time.Time,
    last_activity: time.Time?,
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
