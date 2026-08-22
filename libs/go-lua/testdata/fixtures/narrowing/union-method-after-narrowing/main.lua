type Message = {
    _topic: string,
    topic: (self: Message) -> string
}

type Timer = {elapsed: number}

type MsgCh = {__tag: "msg"}
type TimerCh = {__tag: "timer"}

type Result = {channel: MsgCh, value: Message, ok: boolean} |
              {channel: TimerCh, value: Timer, ok: boolean}

function select_fn(msg_ch: MsgCh, timer_ch: TimerCh): Result
    return {
        channel = msg_ch,
        value = {
            _topic = "test",
            topic = function(s: Message): string return s._topic end
        },
        ok = true
    }
end

function f(msg_ch: MsgCh, timer_ch: TimerCh)
    local result = select_fn(msg_ch, timer_ch)
    if result.channel == timer_ch then
        return nil, "timeout"
    end
    local msg = result.value
    local topic: string = msg:topic()
    return topic
end
