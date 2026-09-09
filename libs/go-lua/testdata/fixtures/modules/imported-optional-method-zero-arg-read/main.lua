local http_client = require("http_client")

local response, err = http_client.get("https://example.test")
if err or not response then
    return nil, err
end

if response.status_code >= 300 and response.stream and not response.body then
    local body_data = response.stream:read()
    response.body = body_data
end

return response
