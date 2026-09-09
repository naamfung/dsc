type Success = {ok: true, value: string}
type Failure = {ok: false, error: string}
type Result = Success | Failure

local function get_value(r: Result): string
    if r.ok then
        return r.value
    end
    return r.error
end
