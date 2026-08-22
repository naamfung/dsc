local process = require("process")

local result, err = process.run("some config input")
if err then
    print("Error: " .. err)
end
if result then
    local msg: string = result.message
    local host: string = result.config.host
    local port: number = result.config.port
    local validated: true = result.config.validated
end
