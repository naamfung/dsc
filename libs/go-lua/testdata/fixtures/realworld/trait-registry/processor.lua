local types = require("types")

local M = {}

function M.normalize_tool(tool_def: types.TraitToolDef): types.TraitToolEntry
    if type(tool_def) == "string" then
        return {id = tool_def}
    end
    local entry: types.TraitToolEntry = {
        id = tool_def.id,
        context = tool_def.context,
        description = tool_def.description,
        alias = tool_def.alias,
    }
    return entry
end

function M.normalize_tools(tools_data: {types.TraitToolDef}?): {types.TraitToolEntry}
    if not tools_data or #tools_data == 0 then
        return {}
    end
    local result: {types.TraitToolEntry} = {}
    for _, tool_def in ipairs(tools_data) do
        table.insert(result, M.normalize_tool(tool_def))
    end
    return result
end

function M.build_trait(entry: types.TraitRegistryEntry): (types.TraitSpec?, string?)
    if not entry.data then
        return nil, "trait has no data: " .. entry.id
    end
    local data = entry.data
    local spec: types.TraitSpec = {
        id = entry.id,
        name = entry.meta and entry.meta.name or entry.id,
        description = entry.meta and entry.meta.comment or "",
        prompt = data.prompt or "",
        tools = M.normalize_tools(data.tools),
        context = data.context or {},
    }
    return spec, nil
end

return M
