type Message = {_topic: string}
type Event = {kind: string}
type Timer = {elapsed: number}

type MsgCh = {__tag: "msg"}
type EventCh = {__tag: "event"}
type TimerCh = {__tag: "timer"}

type Result = {channel: MsgCh, value: Message, ok: boolean} |
              {channel: EventCh, value: Event, ok: boolean} |
              {channel: TimerCh, value: Timer, ok: boolean}

function do_select(m: MsgCh, e: EventCh, t: TimerCh): Result
    return {channel = m, value = {_topic = "test"}, ok = true}
end

function f(msg_ch: MsgCh, events_ch: EventCh, timeout: TimerCh)
    local result = do_select(msg_ch, events_ch, timeout)

    if result.channel == timeout then
        return nil, "timeout"
    end

    if result.channel == events_ch then
        local event = result.value
        local k: string = event.kind
        return "event", k
    end

    local msg = result.value
    local topic: string = msg._topic
    return "message", topic
end
