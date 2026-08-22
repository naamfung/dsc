type Event = {kind: string, from: string, result: any?, error: any?}
type Timer = {elapsed: number}

type EventCh = Channel<Event>
type TimerCh = Channel<Timer>

local M = {}
M.Event = Event

function M.new_event(kind: string, from: string): Event
    return {kind = kind, from = from, result = nil, error = nil}
end

return M
