type Event = {kind: string}
type Timer = {elapsed: number}
type Result = Event | Timer

function get_result(): Result
    return {kind = "exit"}
end

function f()
    local result = get_result()
    local k: string = result.kind -- expect-error
end
