local plugins = require("plugins")

local r = plugins.setup()

local result, err = r:call("greet", {name = "Alice"})
if err == nil and result then
    local output = result.output
end

local result2, err2 = r:call("count", {items = {"a", "b", "c"}})
if err2 == nil and result2 then
    local output = result2.output
end

local missing, missing_err = r:call("nonexistent", {})
if missing_err then
    local msg: string = missing_err
end

local has_greet: boolean = r:has("greet")
local names = r:list()
