type OK = {ok: true, value: string}
type ERR = {ok: false, value: number}
local r: OK | ERR = {ok=true, value="x"}

if r.ok then
    local s: string = r.value
else
    local n: number = r.value
end
