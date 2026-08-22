local time = require("time")
local result = require("result")
local protocol = require("protocol")
local session_store = require("session_store")
local middleware_builder = require("middleware_builder")
local route_builder = require("route_builder")
local router = require("router")

type StringResult = {ok: true, value: string} | {ok: false, error: result.AppError}

local now = time.now()
local store = session_store.new()

store:save("token-1", {
    id = "s-1",
    user_id = "u-1",
    scopes = {["chat.read"] = true},
    last_seen = nil,
    attributes = {role = "owner"},
})

store:save("token-2", {
    id = "s-2",
    user_id = "u-2",
    scopes = {["chat.read"] = false},
    last_seen = now,
    attributes = nil,
})

local observed_users: {string} = {}
local observed_sources: {[string]: string} = {}
local last_timer_body: string? = nil

local auth = middleware_builder.new()
    :named("auth")
    :require_header("authorization")
    :load_sessions_from(store)
    :require_scope("chat.read")
    :copy_tag_to_local("source", "source")
    :build()

local trace = middleware_builder.new()
    :named("trace")
    :copy_tag_to_local("source", "source")
    :build()

local rooms_route = route_builder.new()
    :key("GET /rooms/show")
    :use(auth)
    :require_param("room_id")
    :decorate_body(function(body: string, ctx: protocol.RequestContext): string
        local source = ctx.locals["source"]
        if source then
            return body .. ":" .. source
        end
        return body
    end)
    :handle(function(ctx: protocol.RequestContext): protocol.ResponseResult
        local room_id = ctx.params["room_id"]
        if not room_id then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = "missing room_id",
                    retryable = false,
                },
            }
        end

        local user_id = "guest"
        local freshness = "cold"
        if ctx.session then
            user_id = ctx.session.user_id
            if ctx.session.last_seen then
                freshness = "warm"
            end
        end

        return {
            ok = true,
            value = {
                status = 200,
                body = "room:" .. room_id .. ":" .. user_id .. ":" .. freshness,
                headers = {["x-user"] = user_id},
            },
        }
    end)
    :build()

local health_route = route_builder.new()
    :key("GET /health")
    :handle(function(ctx: protocol.RequestContext): protocol.ResponseResult
        local trace_id = ctx.request.meta.trace_id
        return {
            ok = true,
            value = {
                status = 204,
                body = "health:" .. trace_id,
                headers = {["x-trace"] = trace_id},
            },
        }
    end)
    :build()

local app = router.new()
    :use(trace)
    :register_route(rooms_route)
    :register_route(health_route)

app:on_response(function(ctx: protocol.RequestContext, response: protocol.Response)
    local user = response.headers["x-user"]
    if user then
        table.insert(observed_users, user)
    end

    local source = ctx.locals["source"]
    if source then
        observed_sources[ctx.request.path] = source
    end
end)

local room_request: protocol.HttpRequest = {
    kind = "http",
    method = "GET",
    path = "/rooms/show",
    headers = {authorization = "token-1"},
    params = {room_id = "room-1"},
    body = nil,
    meta = protocol.meta("trace-1", {source = "api"}),
}

local health_request: protocol.HttpRequest = {
    kind = "http",
    method = "GET",
    path = "/health",
    headers = {},
    params = nil,
    body = nil,
    meta = protocol.meta("trace-2", {source = "probe"}),
}

local timer_request: protocol.TimerRequest = {
    kind = "timer",
    id = "tick-1",
    at = now,
    meta = protocol.meta("trace-3", nil),
}

local room_response = app:dispatch(room_request, now)
if room_response.ok then
    local status: integer = room_response.value.status
    local body: string = room_response.value.body
    local maybe_user = room_response.value.headers["x-user"]
    if maybe_user then
        local stable_user: string = maybe_user
    end
end

local room_label = result.and_then(room_response, function(response: protocol.Response): StringResult
    return result.ok(response.body)
end)

if room_label.ok then
    local label: string = room_label.value
end

local health_response = app:dispatch(health_request, now)
if health_response.ok then
    local health_body: string = health_response.value.body
    local trace_header = health_response.value.headers["x-trace"]
    if trace_header then
        local stable_trace: string = trace_header
    end
end

local timer_response = app:dispatch(timer_request, now)
if timer_response.ok then
    last_timer_body = timer_response.value.body
end

if last_timer_body ~= nil then
    local stable_body: string = last_timer_body
end

for _, user in ipairs(observed_users) do
    local stable_user: string = user
end

for path, source in pairs(observed_sources) do
    local stable_path: string = path
    local stable_source: string = source
end

store:touch("token-1", now)
local snapshot = store:lookup("token-1")
if snapshot then
    local seen = snapshot.last_seen or now
    local elapsed = now:sub(seen)
    local seconds: number = elapsed:seconds()

    local attrs = snapshot.attributes
    if attrs then
        local role = attrs["role"]
        if role then
            local stable_role: string = role
        end
    end
end
