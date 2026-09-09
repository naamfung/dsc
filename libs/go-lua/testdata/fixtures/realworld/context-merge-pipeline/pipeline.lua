local context = require("context")

type Stage = {
    name: string,
    process: (ctx: context.Context) -> context.Context,
}

type Pipeline = {
    _stages: {Stage},
    add: (self: Pipeline, name: string, processor: (ctx: context.Context) -> context.Context) -> Pipeline,
    run: (self: Pipeline, initial: context.Context?) -> context.Context,
    count: (self: Pipeline) -> number,
}

local M = {}

function M.new(): Pipeline
    local p: Pipeline = {
        _stages = {},
        add = function(self: Pipeline, name: string, processor: (ctx: context.Context) -> context.Context): Pipeline
            table.insert(self._stages, {name = name, process = processor})
            return self
        end,
        run = function(self: Pipeline, initial: context.Context?): context.Context
            local ctx = initial or context.empty()
            for _, stage in ipairs(self._stages) do
                ctx = stage.process(ctx)
            end
            return ctx
        end,
        count = function(self: Pipeline): number
            return #self._stages
        end,
    }
    return p
end

return M
