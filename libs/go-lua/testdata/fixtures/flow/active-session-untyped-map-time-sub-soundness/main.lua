local time = require("time")

type ActiveSession = {
    created_at: time.Time,
    last_activity: time.Time?,
}

local state = {
    active_sessions = {},
}

local now = time.now()

state.active_sessions["s1"] = {
    created_at = now,
    last_activity = now,
}

for _, session_info in pairs(state.active_sessions) do
    local last_activity = session_info.last_activity or session_info.created_at
    local elapsed = now:sub(last_activity)
    return elapsed:seconds()
end

return 0
