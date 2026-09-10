local protocol = require("protocol")

local M = {}

function M.command_id(command: protocol.Command): string?
    if command.kind == "tick" then
        return nil
    end
    return command.id
end

function M.command_label(command: protocol.Command): string
    if command.kind == "create" then
        return "create:" .. command.customer
    end
    if command.kind == "reserve" then
        return "reserve:" .. command.item_id
    end
    if command.kind == "complete" then
        return "complete"
    end
    return "tick"
end

function M.source_tag(command: protocol.Command): string?
    if command.kind == "tick" then
        return nil
    end
    local tags = command.meta.tags
    if not tags then
        return nil
    end
    return tags["source"]
end

function M.status_name(status: string?): string
    return status or "unknown"
end

return M
