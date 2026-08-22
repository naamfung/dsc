local builder = require("builder")

local b = builder.new("first")
local renamed = b:rename("second")
local name: string = renamed:name()
