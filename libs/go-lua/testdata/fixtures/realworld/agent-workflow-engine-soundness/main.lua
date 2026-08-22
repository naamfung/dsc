local time = require("time")
local engine = require("engine")
local tools = require("tools")
local protocol = require("protocol")

local now = time.now()
local app = engine.new():register_tool("search", tools.search)
local store = app:new_session("unsafe", now)

local msg: protocol.ToolCallMessage = {
    kind = "tool_call",
    id = "m1",
    tool = "search",
    arguments = {topic = "lua"},
    meta = protocol.meta("req-1", "trace-1", nil),
}

local handler = app.handlers["search"]
local produced = handler(store.state, msg) -- expect-error

local cached = store:lookup_tool("search")
local bad_content: string = cached.content -- expect-error

local elapsed = now:sub(store.state.last_activity) -- expect-error
local seconds: number = elapsed:seconds()

local tags = msg.meta.tags
local bad_source: string = tags["source"] -- expect-error
