local time = require("time")

type Snapshot = {
    id: string,
    opened_at: time.Time,
    last_seen: time.Time,
    last_value: string?,
    flags: {[string]: boolean},
}

local M = {}
M.Snapshot = Snapshot

return M
