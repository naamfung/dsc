local counter = require("counter")

local c = counter.new()
c:increment()
c:increment()
c:increment()
c:decrement()

local count: number = c:get()
c:reset()
local zero: number = c:get()

local c2 = counter.new(10)
local ten: number = c2:get()
