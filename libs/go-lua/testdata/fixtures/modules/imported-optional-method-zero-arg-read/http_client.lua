type Stream = {
    read: (self: Stream, n: number?) -> (string?, string?),
}

type Response = {
    status_code: number,
    body: string?,
    stream: Stream?,
}

local http_client = {}

function http_client.get(url: string): (Response?, string?)
    local stream: Stream = {
        read = function(self: Stream, n: number?)
            return "chunk", nil
        end,
    }

    return {
        status_code = 500,
        stream = stream,
    }, nil
end

return http_client
