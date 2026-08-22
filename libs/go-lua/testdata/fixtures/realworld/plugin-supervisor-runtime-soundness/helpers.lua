local protocol = require("protocol")

local M = {}

function M.cache_key(request: protocol.DispatchRequest): string
    return request.plugin .. ":" .. request.envelope.id
end

function M.source_tag(envelope: protocol.PayloadEnvelope): string
    local tags = envelope.meta.tags
    if not tags then
        return "unknown"
    end

    local source = tags["source"]
    if not source then
        return "unknown"
    end

    return source
end

function M.output_label(output: protocol.Output): string
    if output.kind == "rendered" then
        return output.body
    end

    if output.kind == "indexed" then
        return tostring(output.count)
    end

    return output.note
end

return M
