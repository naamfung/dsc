local protocol = require("protocol")

local M = {}

function M.source_tag(call: protocol.PluginCall): string
    local tags = call.meta.tags
    if not tags then
        return "unknown"
    end

    local source = tags["source"]
    if source == nil then
        return "unknown"
    end
    return source
end

function M.policy_label(policy: protocol.RetryPolicy): string
    return policy.label .. ":" .. tostring(policy.max_attempts)
end

function M.status_name(status: string?): string
    if status == nil then
        return "pending"
    end
    return status
end

return M
