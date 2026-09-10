type Store = {
    cache: {[string]: string},
    get: (self: Store, key: string) -> string?,
    put: (self: Store, key: string, value: string) -> Store,
}

local Store = {}
Store.__index = Store

local M = {}
M.Store = Store

function M.new(): Store
    local self: Store = {
        cache = {},
        get = Store.get,
        put = Store.put,
    }
    setmetatable(self, Store)
    return self
end

function Store:get(key: string): string?
    return self.cache[key]
end

function Store:put(key: string, value: string): Store
    self.cache[key] = value
    return self
end

return M
