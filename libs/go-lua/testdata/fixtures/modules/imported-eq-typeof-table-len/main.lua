local test = require("test")

local values: {string}? = { "alpha", "beta" }

test.eq(type(values), "table", "values should be a table")

local count: number = #values
local first: string = values[1]

return count, first
