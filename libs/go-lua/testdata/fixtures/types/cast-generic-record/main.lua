type StringResult = {ok: boolean, value: string}
local data: any = {ok = true, value = "success"}
local r = StringResult(data)
local v = r.value
