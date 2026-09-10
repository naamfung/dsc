type Snapshot = {
    id: string,
    last_value: string?,
    flags: {[string]: boolean},
}

local M = {}
M.Snapshot = Snapshot

return M
