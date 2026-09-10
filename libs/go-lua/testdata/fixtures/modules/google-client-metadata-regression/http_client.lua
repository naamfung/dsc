local http_client = {}

type StreamReader = {
    read: (self: any) -> (string?, string?),
}

type Response = {
    status_code: number,
    body: string?,
    stream: StreamReader?,
    headers: {[string]: string}?,
}

function http_client.get(url: string, options: any?): (Response?, string?)
    return nil, "not implemented"
end

function http_client.post(url: string, options: any?): (Response?, string?)
    return nil, "not implemented"
end

function http_client.put(url: string, options: any?): (Response?, string?)
    return nil, "not implemented"
end

function http_client.patch(url: string, options: any?): (Response?, string?)
    return nil, "not implemented"
end

return http_client
