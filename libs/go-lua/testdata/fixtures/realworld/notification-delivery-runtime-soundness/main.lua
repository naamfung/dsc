local time = require("time")
local protocol = require("protocol")
local template_builder = require("template_builder")
local transport_builder = require("transport_builder")
local runtime = require("runtime")

local now = time.now()

local email_renderer = template_builder.new()
    :named("email")
    :prefix_with("subject")
    :require_tag("source")
    :build()

local email_transport = transport_builder.new()
    :for_channel("email")
    :use_renderer(email_renderer)
    :count_as("mailops")
    :require_tag("source")
    :build()

local app = runtime.new():register_transport("email", email_transport)

local email_one: protocol.EmailRequest = {
    kind = "email",
    tenant_id = "tenant-a",
    message_id = "msg-1",
    recipient = "alice@example.com",
    subject = "welcome",
    template = "welcome-email",
    meta = protocol.meta("trace-1", nil),
}

local tick: protocol.TickRequest = {
    kind = "tick",
    at = now,
}

local store = app:new_store("delivery-soundness", now)
local run_result = app:run(store, {tick}, now)

if run_result.ok then
    local last_status: string = run_result.value.last_status -- expect-error
end

local record: protocol.DeliveryRecord = store.state.records["missing"] -- expect-error
local receipt: protocol.DeliveryReceipt = store.state.cached_receipts["missing"] -- expect-error
local handler: protocol.TransportHandler = app.transports["email"] -- expect-error

local elapsed = now:sub(store.state.last_delivery_at) -- expect-error
local source: string = email_one.meta.tags["source"] -- expect-error

local missing_status: string = store:lookup_record("msg-1").last_status -- expect-error
