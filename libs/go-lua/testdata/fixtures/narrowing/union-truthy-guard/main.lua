type Event = {kind: string, error: string?}
type Timer = {elapsed: number}
type SelectResult = Event | Timer

function get_result(): SelectResult
    return {kind = "exit", error = nil}
end

function f()
    local result = get_result()
    if result.kind then
        local k: string = result.kind
    end
end
