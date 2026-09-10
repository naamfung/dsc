type Counter = {
    count: number,
    increment: (self: Counter) -> number
}
local c: Counter = {
    count = 0,
    increment = function(self: Counter): number
        self.count = self.count + 1
        return self.count
    end
}
local n: number = c:increment()
