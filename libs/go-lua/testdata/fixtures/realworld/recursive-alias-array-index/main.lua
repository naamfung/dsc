type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

local function make(): {Message}
    return {{
        _topic = "indexed",
        topic = function(self: Message): string
            return self._topic
        end,
    }}
end

local topic: string = make()[1]:topic()
