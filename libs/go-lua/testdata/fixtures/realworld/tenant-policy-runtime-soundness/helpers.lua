local protocol = require("protocol")

local M = {}

function M.request_label(request: protocol.Request): string
    if request.kind == "auth" then
        return "auth:" .. request.scope
    end
    if request.kind == "query" then
        return "query:" .. request.resource
    end
    if request.kind == "update" then
        return "update:" .. request.resource
    end
    return "tick"
end

function M.tag_value(request: protocol.Request, key: string): string?
    if request.kind == "tick" then
        return nil
    end

    local tags = request.meta.tags
    if not tags then
        return nil
    end
    return tags[key]
end

function M.decision_note(decision: protocol.Decision): string
    if decision.kind == "allow" then
        return "allow:" .. decision.reason
    end
    if decision.kind == "deny" then
        return "deny:" .. decision.reason
    end
    return "defer:" .. decision.queue
end

function M.bump_counter(counters: {[string]: integer}, key: string)
    local current = counters[key]
    if current then
        counters[key] = current + 1
        return
    end
    counters[key] = 1
end

return M
