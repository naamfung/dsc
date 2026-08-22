type Result<T, E> = {ok: true, value: T} | {ok: false, error: E}
local r: Result<number, string> = {ok = true, value = 42}
