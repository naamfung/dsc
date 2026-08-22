local time = require("time")
local protocol = require("protocol")

local M = {}

function M.make(id: string, now: time.Time): protocol.Snapshot
    return {
        id = id,
        opened_at = now,
        last_seen = now,
        last_value = nil,
    }
end

return M
