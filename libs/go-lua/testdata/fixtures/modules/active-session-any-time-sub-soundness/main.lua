local time = require("time")
local session_state = require("session_state")

local now = time.now()
local session_info = session_state.new()
local last_activity = session_info.last_activity or session_info.created_at
local elapsed = now:sub(last_activity)

return elapsed:seconds()
