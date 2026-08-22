type Container<T> = {
    _value: T,
    get: (self: Container<T>) -> T
}
local c: Container<number> = {
    _value = 42,
    get = function(self: Container<number>): number
        return self._value
    end
}
local n: number = c:get()
