local test = require("test")
local client = require("client")

local response, err = client.request(true)

test.is_nil(err, "no error expected")

local id: string = response.metadata.response_id

return id
