type Counter = {
    count: number,
    increment: (self: Counter) -> (),
    decrement: (self: Counter) -> (),
    get: (self: Counter) -> number,
    reset: (self: Counter) -> ()
}

local M = {}

function M.new(initial: number?): Counter
    local c = {
        count = initial or 0,
        increment = function(self: Counter)
            self.count = self.count + 1
        end,
        decrement = function(self: Counter)
            self.count = self.count - 1
        end,
        get = function(self: Counter): number
            return self.count
        end,
        reset = function(self: Counter)
            self.count = 0
        end
    }
    return c
end

return M
