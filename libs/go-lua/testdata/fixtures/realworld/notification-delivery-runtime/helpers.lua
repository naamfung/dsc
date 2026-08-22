local protocol = require("protocol")

local M = {}

function M.request_label(request: protocol.Request): string
    if request.kind == "email" then
        return "email:" .. request.subject
    end
    if request.kind == "sms" then
        return "sms:" .. request.phone
    end
    if request.kind == "webhook" then
        return "webhook:" .. request.endpoint
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

function M.receipt_note(receipt: protocol.DeliveryReceipt): string
    return receipt.channel .. ":" .. receipt.local_status .. ":" .. receipt.provider_id
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
