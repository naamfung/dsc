local mapper = require("mapper")

local reason: string = mapper.map_finish_reason("end_turn")
local err_type: string = mapper.map_error_type("rate_limit_error")
local status_type: string = mapper.map_status_code(429)

local direct: string = mapper.finish_reasons["end_turn"]
local direct_err: string = mapper.error_types["api_error"]
