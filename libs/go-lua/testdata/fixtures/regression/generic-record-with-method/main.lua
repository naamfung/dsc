type Container<T> = {
    value: T,
    get: (self: Container<T>) -> T
}
local c: Container<string> = {
    value = "hello",
    get = function(self: Container<string>): string
        return self.value
    end
}
local s: string = c:get()
