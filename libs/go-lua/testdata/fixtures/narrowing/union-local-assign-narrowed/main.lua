type EventCh = {__tag: "event"}
type TimeoutCh = {__tag: "timeout"}
type Event = {kind: string, error: string?}
type Time = {sec: number}

type Result = {channel: EventCh, value: Event, ok: boolean} |
              {channel: TimeoutCh, value: Time, ok: boolean}

function get_result(ch: EventCh, timeout: TimeoutCh): Result
    return {channel = ch, value = {kind = "exit", error = nil}, ok = true}
end

function f(events_ch: EventCh, timeout_ch: TimeoutCh)
    local result = get_result(events_ch, timeout_ch)
    if result.channel ~= events_ch then
        return false, "timeout"
    end
    local event = result.value
    local k: string = event.kind
    if event.error then
        local e: string = event.error
    end
    return true
end
