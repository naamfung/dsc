type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

type Result = {
    message: Message,
}

local function make(): Result
    return {
        message = {
            _topic = "test",
            topic = function(self: Message): string
                return self._topic
            end,
        },
    }
end

local topic: string = make().message:topic()
