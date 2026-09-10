type Context = {[string]: any}
type ContextMerger = (base: Context?, override: Context?) -> Context
type Middleware = (ctx: Context, next: (Context) -> Context) -> Context

local M = {}
M.Context = Context

function M.merge(base: Context?, override: Context?): Context
    local merged: Context = {}
    if base then
        for k, v in pairs(base) do
            merged[k] = v
        end
    end
    if override then
        for k, v in pairs(override) do
            merged[k] = v
        end
    end
    return merged
end

function M.empty(): Context
    return {}
end

function M.with(ctx: Context, key: string, value: any): Context
    local new_ctx = M.merge(ctx, nil)
    new_ctx[key] = value
    return new_ctx
end

function M.get(ctx: Context, key: string): any
    return ctx[key]
end

return M
