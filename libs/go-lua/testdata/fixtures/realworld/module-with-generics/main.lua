local collection = require("collection")

local nums = collection.new()
nums:add(1)
nums:add(2)
nums:add(3)
local count: number = nums:count()
local first = nums:get(1)

local names = collection.new()
names:add("alice")
names:add("bob")
local name_count: number = names:count()
