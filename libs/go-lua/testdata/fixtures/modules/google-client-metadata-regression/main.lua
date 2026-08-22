local client = require("client")
local json = require("json")
local tests = require("tests")

client._http_client = {
    get = function(url, options)
        return {
            status_code = 200,
            body = json.encode({
                data = "test",
                modelVersion = "gemini-2.5-pro-001",
                responseId = "resp-123",
                createTime = "2024-01-15T10:30:00Z",
            })
        }
    end
}

local response, err = client.request("GET", "https://test.googleapis.com/v1/test", {
    headers = {}
})

tests.is_nil(err)
tests.eq(response.metadata.model_version, "gemini-2.5-pro-001")
tests.eq(response.metadata.response_id, "resp-123")
tests.eq(response.metadata.create_time, "2024-01-15T10:30:00Z")

return response.data
