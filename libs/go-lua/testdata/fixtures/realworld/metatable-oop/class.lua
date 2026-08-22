type EventHandler = (self: EventEmitter, event: string, data: any) -> ()

type EventEmitter = {
    _handlers: {[string]: {EventHandler}},
    on: (self: EventEmitter, event: string, handler: EventHandler) -> EventEmitter,
    emit: (self: EventEmitter, event: string, data: any) -> (),
}

local EventEmitter = {}
EventEmitter.__index = EventEmitter

function EventEmitter.new(): EventEmitter
    local self: EventEmitter = {
        _handlers = {},
        on = EventEmitter.on,
        emit = EventEmitter.emit,
    }
    setmetatable(self, EventEmitter)
    return self
end

function EventEmitter:on(event: string, handler: EventHandler): EventEmitter
    if not self._handlers[event] then
        self._handlers[event] = {}
    end
    table.insert(self._handlers[event], handler)
    return self
end

function EventEmitter:emit(event: string, data: any)
    local handlers = self._handlers[event]
    if handlers then
        for _, handler in ipairs(handlers) do
            handler(self, event, data)
        end
    end
end

local M = {}
M.EventEmitter = EventEmitter
M.new = EventEmitter.new
return M
