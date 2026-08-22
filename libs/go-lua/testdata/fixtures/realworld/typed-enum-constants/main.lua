local status = require("status")
local handler = require("handler")

local router = handler.new()
    :add("GET", "/users", function(req: status.Request): status.Response
        return status.ok({users = {"Alice", "Bob"}})
    end)
    :add("POST", "/users", function(req: status.Request): status.Response
        if not req.body then
            return status.error(400, "Missing body")
        end
        return status.created({id = "new-user"})
    end)
    :add("DELETE", "/users", function(req: status.Request): status.Response
        return status.ok()
    end)

local get_resp = router:handle({
    method = "GET",
    path = "/users",
    headers = {},
})
local get_status: number = get_resp.status

local post_resp = router:handle({
    method = "POST",
    path = "/users",
    body = {name = "Charlie"},
    headers = {["content-type"] = "application/json"},
})

local not_found = router:handle({
    method = "GET",
    path = "/missing",
    headers = {},
})
local nf_status: number = not_found.status
