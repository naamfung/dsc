local protocol = require("protocol")

local M = {}

function M.make(id: string): protocol.Snapshot
    return {
        id = id,
        flags = {},
    }
end

return M
