type Handler = (args: {[string]: any}) -> (any?, string?)

type Registry = {
    _entries: {[string]: Handler},
    register: (self: Registry, name: string, handler: Handler) -> (),
    call: (self: Registry, name: string, args: {[string]: any}) -> (any?, string?),
    has: (self: Registry, name: string) -> boolean,
    list: (self: Registry) -> {string},
}

local M = {}
M.Registry = Registry

function M.new(): Registry
    local r: Registry = {
        _entries = {},
        register = function(self: Registry, name: string, handler: Handler)
            self._entries[name] = handler
        end,
        call = function(self: Registry, name: string, args: {[string]: any}): (any?, string?)
            local handler = self._entries[name]
            if not handler then
                return nil, "handler not found: " .. name
            end
            return handler(args)
        end,
        has = function(self: Registry, name: string): boolean
            return self._entries[name] ~= nil
        end,
        list = function(self: Registry): {string}
            local names: {string} = {}
            for name, _ in pairs(self._entries) do
                table.insert(names, name)
            end
            return names
        end,
    }
    return r
end

return M
