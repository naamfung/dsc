type Collection<T> = {
    items: {T},
    count: (self: Collection<T>) -> number,
    get: (self: Collection<T>, index: integer) -> T?,
    add: (self: Collection<T>, item: T) -> ()
}

local M = {}

function M.new<T>(): Collection<T>
    local c: Collection<T> = {
        items = {},
        count = function(self: Collection<T>): number
            return #self.items
        end,
        get = function(self: Collection<T>, index: integer): T?
            return self.items[index]
        end,
        add = function(self: Collection<T>, item: T)
            table.insert(self.items, item)
        end
    }
    return c
end

return M
