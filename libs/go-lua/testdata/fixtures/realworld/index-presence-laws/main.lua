type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

local messages: {[string]: Message} = {}

if not messages["root"] then
    messages["root"] = {
        _topic = "installed",
        topic = function(self: Message): string
            return self._topic
        end,
    }
end

local installed: string = messages["root"]:topic()

local cached = messages["root"]
if cached then
    local cached_topic: string = cached:topic()
end

assert(messages["root"])
local asserted: string = messages["root"]:topic()
