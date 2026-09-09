local types = require("types")

type HandlerResult = {ok: boolean, message: string}

local M = {}

function M.process_event(event: types.Event): HandlerResult
    if event.error then
        return {ok = false, message = "error: " .. tostring(event.error)}
    end
    if event.kind == "EXIT" then
        return {ok = true, message = "exit from " .. event.from}
    end
    return {ok = true, message = "processed " .. event.kind}
end

return M
