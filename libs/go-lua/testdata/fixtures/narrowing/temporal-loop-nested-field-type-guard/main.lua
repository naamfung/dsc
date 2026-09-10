type Message = {
    from: fun(self: Message): string,
    payload: fun(self: Message): any,
}

type Channel = {
    receive: fun(self: Channel): (Message, boolean),
}

local process = {}

function process.listen(topic: string, options: any?): Channel
    error("stub")
end

function process.send(pid: string, topic: string, ...: any): (boolean, string?)
    return true, nil
end

local counter = 0
local done = false

coroutine.spawn(function()
    local ch = process.listen("increment", {message = true})
    while not done do
        local msg, ok = ch:receive()
        if not ok then
            break
        end

        local p = msg:payload()
        local data = p and p:data() or nil
        local reply_to = msg:from()

        if type(data) ~= "table" or type(data.amount) ~= "number" then
            process.send(reply_to, "nak", "amount must be a number")
        else
            process.send(reply_to, "ack")
            local amount_sanity = data.amount + 1
            counter = counter + data.amount
            counter = amount_sanity - 1
        end
    end
end)
