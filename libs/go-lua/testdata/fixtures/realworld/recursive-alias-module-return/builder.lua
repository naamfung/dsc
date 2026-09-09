type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

type Result = {
    message: Message,
}

local M = {}

function M.make(): Result
    return {
        message = {
            _topic = "exported",
            topic = function(self: Message): string
                return self._topic
            end,
        },
    }
end

return M
