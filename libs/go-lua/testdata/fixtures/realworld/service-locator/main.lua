local locator = require("locator")

local services = locator.init("debug")

services.logger:info("starting up")
services.cache:set("session", "abc123", 3600)

local log = locator.logger()
log:debug("cache populated")

local c = locator.cache()
local has_session: boolean = c:has("session")
local session = c:get("session")

c:delete("session")
c:clear()
