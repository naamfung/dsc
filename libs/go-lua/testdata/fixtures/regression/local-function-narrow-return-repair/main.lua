local function f(blocks)
    local tool_use_block = nil
    for _, block in ipairs(blocks) do
        if block.type == "tool_use" and block.name == "structured_output" then
            tool_use_block = block
            break
        end
    end
    if not tool_use_block then
        return { success = false, error = "missing" }
    end
    return { success = true, result = { data = tool_use_block.input } }
end

local export = { f = f }

local out = export.f({
    {
        type = "tool_use",
        name = "structured_output",
        input = { "ok" },
    },
})

if out.success and type(out.result.data) == "table" then
    local n: integer = #out.result.data
end

return export
