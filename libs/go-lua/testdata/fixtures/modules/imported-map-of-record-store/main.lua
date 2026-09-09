local protocol = require("protocol")
local store_mod = require("store")

local store: store_mod.Store = store_mod.new()
store:put("s1", {
    id = "s1",
    last_value = nil,
    flags = {},
})

local snapshot = store:get("s1")
if snapshot then
    local id: string = snapshot.id
    local ready = snapshot.flags["ready"]
end
