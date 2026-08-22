local time = require("time")
local result = require("result")
local protocol = require("protocol")
local session_store = require("session_store")
local middleware_builder = require("middleware_builder")
local route_builder = require("route_builder")
local router = require("router")

local now = time.now()
local store = session_store.new()

store:save("token-1", {
    id = "s-1",
    user_id = "u-1",
    scopes = {["chat.read"] = true},
    last_seen = nil,
    attributes = nil,
})

local auth = middleware_builder.new()
    :named("auth")
    :require_header("authorization")
    :load_sessions_from(store)
    :require_scope("chat.read")
    :copy_tag_to_local("source", "source")
    :build()

local unsafe_route = route_builder.new()
    :key("GET /rooms/show")
    :use(auth)
    :handle(function(ctx: protocol.RequestContext): protocol.ResponseResult
        local user_id: string = ctx.session.user_id -- expect-error
        local room_id: string = ctx.params["room_id"] -- expect-error
        return {
            ok = true,
            value = {
                status = 200,
                body = user_id .. ":" .. room_id,
                headers = {["x-user"] = user_id},
            },
        }
    end)
    :build()

local app = router.new():register_route(unsafe_route)

local room_request: protocol.HttpRequest = {
    kind = "http",
    method = "GET",
    path = "/rooms/show",
    headers = {authorization = "token-1"},
    params = {room_id = "room-1"},
    body = nil,
    meta = protocol.meta("trace-1", nil),
}

local response = app:dispatch(room_request, now)
if response.ok then
    local header: string = response.value.headers["x-user"] -- expect-error
end

local snapshot = store:lookup("token-1")
if snapshot then
    local elapsed = now:sub(snapshot.last_seen) -- expect-error
end

local tags = room_request.meta.tags
local source: string = tags["source"] -- expect-error

local request_room: string = room_request.params["room_id"] -- expect-error
