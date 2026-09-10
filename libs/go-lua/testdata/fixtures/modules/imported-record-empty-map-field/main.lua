local protocol = require("protocol")
local store = require("store")

local snapshot: protocol.Snapshot = store.make("s1")
snapshot.flags["ready"] = true
