type EventCh = {__tag: "event"}
type TimeoutCh = {__tag: "timeout"}
type Event = {kind: string}
type Time = {sec: number}

type Result = {channel: EventCh, value: Event, ok: boolean} |
              {channel: TimeoutCh, value: Time, ok: boolean}

function get_result(ch: EventCh, timeout: TimeoutCh): Result
    return {channel = ch, value = {kind = "exit"}, ok = true}
end

function f(events_ch: EventCh, timeout_ch: TimeoutCh)
    local result = get_result(events_ch, timeout_ch)
    if result.channel ~= events_ch then
        local t: Time = result.value
        return false
    end
    local event: Event = result.value
    return true
end
