local types = require("types")
local handler = require("handler")

local event = types.new_event("EXIT", "test")
local result = handler.process_event(event)

local ok: boolean = result.ok
local msg: string = result.message
