type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

local function make(): {[string]: Message}
    return {
        ["root"] = {
            _topic = "mapped",
            topic = function(self: Message): string
                return self._topic
            end,
        },
    }
end

local root = make()["root"]
if root then
    local topic: string = root:topic()
end
