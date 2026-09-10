local function fetch(url: string, on_done: (string) -> nil)
    on_done("response")
end
fetch("http://example.com", function(data: string)
    local s: string = data
end)
