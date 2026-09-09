local time = require("time")
local protocol = require("protocol")
local store = require("store")

local now = time.now()
local snapshot: protocol.Snapshot = store.make("s1", now)

local label: string = snapshot.id
local opened = snapshot.opened_at
local elapsed = now:sub(opened)
local seconds: number = elapsed:seconds()

if snapshot.last_value then
    local value: string = snapshot.last_value
end
