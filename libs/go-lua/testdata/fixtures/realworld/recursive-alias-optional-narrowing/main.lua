type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

type Result = {
    message: Message?,
}

local function make(ok: boolean): Result
    if ok then
        return {
            message = {
                _topic = "optional",
                topic = function(self: Message): string
                    return self._topic
                end,
            },
        }
    end
    return {message = nil}
end

local result = make(true)
if result.message then
    local topic: string = result.message:topic()
end
