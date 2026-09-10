local protocol = require("protocol")

local M = {}

function M.source_tag(msg: protocol.ToolCallMessage): string
    local tags = msg.meta.tags
    if not tags then
        return "unknown"
    end

    local source = tags["source"]
    if source == nil then
        return "unknown"
    end
    return source
end

function M.cache_mode(state: protocol.SessionState): string
    local status = "first"
    if state.flags["profile_loaded"] then
        status = "repeat"
    end
    return status
end

return M
