local class = require("class")

type Counter = {
    _count: number,
    _name: string,
    _emitter: class.EventEmitter,
    increment: (self: Counter) -> (),
    decrement: (self: Counter) -> (),
    get: (self: Counter) -> number,
    name: (self: Counter) -> string,
    on_change: (self: Counter, handler: (self: class.EventEmitter, event: string, data: any) -> ()) -> Counter,
}

local Counter = {}
Counter.__index = Counter

function Counter.new(name: string, initial: number?): Counter
    local emitter = class.new()
    local self: Counter = {
        _count = initial or 0,
        _name = name,
        _emitter = emitter,
        increment = Counter.increment,
        decrement = Counter.decrement,
        get = Counter.get,
        name = Counter.name,
        on_change = Counter.on_change,
    }
    setmetatable(self, Counter)
    return self
end

function Counter:increment()
    self._count = self._count + 1
    self._emitter:emit("change", {value = self._count, delta = 1})
end

function Counter:decrement()
    self._count = self._count - 1
    self._emitter:emit("change", {value = self._count, delta = -1})
end

function Counter:get(): number
    return self._count
end

function Counter:name(): string
    return self._name
end

function Counter:on_change(handler: (self: class.EventEmitter, event: string, data: any) -> ()): Counter
    self._emitter:on("change", handler)
    return self
end

local M = {}
M.new = Counter.new
return M
