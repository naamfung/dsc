type Event = {kind: string, from: string, result: any?, error: any?}
type Timer = {elapsed: number}

type EventCh = {__tag: "event"}
type TimerCh = {__tag: "timer"}

type SelectResult = {channel: EventCh, value: Event, ok: boolean} |
                    {channel: TimerCh, value: Timer, ok: boolean}

function do_select(events: EventCh, timeout: TimerCh): SelectResult
    return {channel = events, value = {kind = "EXIT", from = "test", result = nil, error = nil}, ok = true}
end

function f(events_ch: EventCh)
    local timeout: TimerCh = {__tag = "timer"}
    local result = do_select(events_ch, timeout)

    if result.channel == timeout then
        return false, "timeout"
    end

    local event = result.value
    if event.kind ~= "EXIT" then
        return false, "wrong event"
    end
    if event.error then
        return false, "error"
    end
    return true
end
