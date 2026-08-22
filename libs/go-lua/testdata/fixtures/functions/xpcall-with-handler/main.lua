local ok, result = xpcall(function() return "test" end, function(err) return err end)
local b: boolean = ok
