type Result = {ok: boolean, data: any}
local function fetch(): Result
    return {ok = true, data = "hello"}
end
local r = fetch()
local ok: boolean = r.ok
