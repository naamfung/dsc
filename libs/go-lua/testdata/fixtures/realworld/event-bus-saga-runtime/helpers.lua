local protocol = require("protocol")

local M = {}

function M.event_label(event: protocol.Event): string
    if event.kind == "queued" then
        return "queued:" .. event.queue
    end
    if event.kind == "started" then
        return "started:" .. event.worker
    end
    if event.kind == "completed" then
        return "completed"
    end
    if event.kind == "failed" then
        return "failed:" .. event.code
    end
    return "tick"
end

function M.event_id(event: protocol.Event): string?
    if event.kind == "tick" then
        return nil
    end
    return event.id
end

function M.source_tag(event: protocol.Event): string?
    if event.kind == "tick" then
        return nil
    end
    local tags = event.meta.tags
    if not tags then
        return nil
    end
    return tags["source"]
end

function M.status_name(status: string?): string
    return status or "unknown"
end

return M
