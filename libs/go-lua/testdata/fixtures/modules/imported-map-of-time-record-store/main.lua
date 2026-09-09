local time = require("time")
local store_mod = require("store")

local now = time.now()
local store: store_mod.Store = store_mod.new()
local snapshot = store:open("s1", now)
local copy = store:get("s1")
if copy then
    local id: string = copy.id
end
