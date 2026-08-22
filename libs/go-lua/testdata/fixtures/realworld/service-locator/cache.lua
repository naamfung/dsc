type CacheEntry<T> = {value: T, expires_at: number}

type Cache = {
    _store: {[string]: any},
    get: (self: Cache, key: string) -> any?,
    set: (self: Cache, key: string, value: any, ttl: number?) -> (),
    delete: (self: Cache, key: string) -> (),
    has: (self: Cache, key: string) -> boolean,
    clear: (self: Cache) -> (),
}

local M = {}
M.Cache = Cache

function M.new(): Cache
    local c: Cache = {
        _store = {},
        get = function(self: Cache, key: string): any?
            local entry = self._store[key]
            if not entry then return nil end
            return entry.value
        end,
        set = function(self: Cache, key: string, value: any, ttl: number?)
            self._store[key] = {value = value, expires_at = ttl or 0}
        end,
        delete = function(self: Cache, key: string)
            self._store[key] = nil
        end,
        has = function(self: Cache, key: string): boolean
            return self._store[key] ~= nil
        end,
        clear = function(self: Cache)
            self._store = {}
        end,
    }
    return c
end

return M
