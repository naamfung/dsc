local counter = require("counter")

local c = counter.new("hits", 0)

c:on_change(function(self, event, data)
    local val = data.value
end)

c:increment()
c:increment()
c:decrement()

local count: number = c:get()
local name: string = c:name()
