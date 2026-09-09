type Event = {kind: string, error: string?}
type Message = {topic: string, payload: any}
type Timer = {elapsed: number}
type SelectResult = Event | Message | Timer

local function get_result(): SelectResult
    return {kind = "exit", error = nil}
end

local function f()
    local result = get_result()
    if result.kind then
        local k: string = result.kind
    end
end
