local registry = require("registry")

type PluginConfig = {name: string, version: string, enabled: boolean}
type PluginResult = {output: string, metadata: {[string]: any}}

local M = {}

function M.setup(): registry.Registry
    local r = registry.new()

    r:register("greet", function(args: {[string]: any}): (PluginResult?, string?)
        local name = args.name
        if not name then
            return nil, "name is required"
        end
        return {output = "Hello, " .. tostring(name), metadata = {greeted = true}}, nil
    end)

    r:register("count", function(args: {[string]: any}): (PluginResult?, string?)
        local items = args.items
        if not items then
            return nil, "items is required"
        end
        local n = #items
        return {output = tostring(n) .. " items", metadata = {count = n}}, nil
    end)

    return r
end

return M
