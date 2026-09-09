local config = require("config")

local cfg = config.new()
    :host("example.com")
    :port(9090)
    :debug(true)
    :tag("production")
    :tag("v2")
    :build()

local host: string = cfg.host
local port: number = cfg.port
local debug_mode: boolean = cfg.debug
