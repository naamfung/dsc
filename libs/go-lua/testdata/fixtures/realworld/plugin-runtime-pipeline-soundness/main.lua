local time = require("time")
local result = require("result")
local protocol = require("protocol")
local runtime = require("runtime")

local now = time.now()
local app = runtime.new()

local policy: protocol.RetryPolicy = {
    label = "single",
    max_attempts = 1,
    compute_delay = function(attempt: integer): number
        return attempt
    end,
    should_retry = function(_error: result.AppError, _attempt: integer): boolean
        return false
    end,
}

local store = app:new_store("runtime-unsafe", now)
local call: protocol.PluginCall = {
    kind = "plugin_call",
    id = "p1",
    plugin = "missing",
    input = {query = "lua"},
    meta = protocol.meta("trace-unsafe", {source = "planner"}),
}

local maybe_handler = app.handlers["missing"]
local produced = maybe_handler(store.state, call, policy) -- expect-error

local cached = store:lookup("search")
local bad_content: string = cached.content -- expect-error

local elapsed = now:sub(store.state.last_seen) -- expect-error
local seconds: number = elapsed:seconds()

local hook_step: protocol.RuntimeStep = {
    kind = "hook",
    name = "retry",
    detail = "retry",
}

local tags = call.meta.tags
local bad_source: string = tags["source"] -- expect-error
