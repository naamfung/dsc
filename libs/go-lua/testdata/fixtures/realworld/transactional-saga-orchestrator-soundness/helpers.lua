local protocol = require("protocol")

local M = {}

function M.action_id(action: protocol.Action): string?
    if action.kind == "tick" then
        return nil
    end
    return action.order_id
end

function M.action_label(action: protocol.Action): string
    if action.kind == "begin" then
        return "begin:" .. action.customer_id
    end
    if action.kind == "reserve" then
        return "reserve:" .. action.sku
    end
    if action.kind == "charge" then
        return "charge:" .. tostring(action.cents)
    end
    if action.kind == "commit" then
        return "commit"
    end
    if action.kind == "cancel" then
        return "cancel:" .. action.reason
    end
    return "tick"
end

function M.source_tag(action: protocol.Action): string?
    if action.kind == "tick" then
        return nil
    end
    local tags = action.meta.tags
    if not tags then
        return nil
    end
    return tags["source"]
end

function M.status_name(status: string?): string
    return status or "unknown"
end

return M
