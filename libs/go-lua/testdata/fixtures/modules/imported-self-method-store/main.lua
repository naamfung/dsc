local store_mod = require("store")

local store: store_mod.Store = store_mod.new()
store:put("name", "lua")

local maybe_name = store:get("name")
if maybe_name then
    local value: string = maybe_name
end
