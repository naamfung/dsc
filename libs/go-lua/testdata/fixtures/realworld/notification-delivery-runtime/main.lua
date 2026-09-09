local time = require("time")
local result = require("result")
local protocol = require("protocol")
local helpers = require("helpers")
local template_builder = require("template_builder")
local transport_builder = require("transport_builder")
local runtime = require("runtime")

type StringResult = {ok: true, value: string} | {ok: false, error: result.AppError}

local now = time.now()

local observed_deliveries: {[string]: string} = {}
local observed_retries: {string} = {}
local observed_audits: {string} = {}
local last_runtime_id: string? = nil

local email_renderer = template_builder.new()
    :named("email")
    :prefix_with("subject")
    :require_tag("source")
    :suffix_with("mail")
    :decorate(function(body: string, request: protocol.Request): string
        if request.kind == "email" then
            return body .. ":" .. request.tenant_id
        end
        return body
    end)
    :build()

local sms_renderer = template_builder.new()
    :named("sms")
    :prefix_with("sms")
    :suffix_with("text")
    :decorate(function(body: string, request: protocol.Request): string
        if request.kind == "sms" then
            return body .. ":" .. request.tenant_id
        end
        return body
    end)
    :build()

local webhook_renderer = template_builder.new()
    :named("webhook")
    :prefix_with("json")
    :require_tag("source")
    :suffix_with("hook")
    :decorate(function(body: string, request: protocol.Request): string
        if request.kind == "webhook" then
            return body .. ":" .. request.tenant_id
        end
        return body
    end)
    :build()

local email_transport = transport_builder.new()
    :for_channel("email")
    :use_renderer(email_renderer)
    :count_as("mailops")
    :require_tag("source")
    :decorate_preview(function(preview: string, request: protocol.Request): string
        return preview .. ":" .. helpers.request_label(request)
    end)
    :build()

local sms_transport = transport_builder.new()
    :for_channel("sms")
    :use_renderer(sms_renderer)
    :count_as("smsops")
    :decorate_preview(function(preview: string, request: protocol.Request): string
        return preview .. ":" .. helpers.request_label(request)
    end)
    :build()

local webhook_transport = transport_builder.new()
    :for_channel("webhook")
    :use_renderer(webhook_renderer)
    :count_as("hookops")
    :require_tag("source")
    :decorate_preview(function(preview: string, request: protocol.Request): string
        return preview .. ":" .. helpers.request_label(request)
    end)
    :build()

local app = runtime.new()
    :register_transport("email", email_transport)
    :register_transport("sms", sms_transport)
    :register_transport("webhook", webhook_transport)

app:on_step(function(step: protocol.DeliveryStep, state: protocol.StoreState)
    last_runtime_id = state.id
    if step.kind == "delivery" then
        observed_deliveries[step.message_id] = step.note
        local provider_id: string = step.provider_id
    elseif step.kind == "retry" then
        table.insert(observed_retries, step.note)
        local retry_seconds: integer = step.retry_at:unix()
    else
        table.insert(observed_audits, step.note)
        local at_seconds: integer = step.at:unix()
    end
end)

local email_one: protocol.EmailRequest = {
    kind = "email",
    tenant_id = "tenant-a",
    message_id = "msg-1",
    recipient = "alice@example.com",
    subject = "welcome",
    template = "welcome-email",
    meta = protocol.meta("trace-1", {source = "api", priority = "high"}),
}

local sms_one: protocol.SmsRequest = {
    kind = "sms",
    tenant_id = "tenant-a",
    message_id = "msg-2",
    phone = "+155555501",
    template = "otp",
    meta = protocol.meta("trace-2", {source = "cron"}),
}

local webhook_one: protocol.WebhookRequest = {
    kind = "webhook",
    tenant_id = "tenant-b",
    message_id = "msg-3",
    endpoint = "https://example.com/hook",
    template = "sync",
    meta = protocol.meta("trace-3", {source = "worker", retry = "true", priority = "low"}),
}

local tick: protocol.TickRequest = {
    kind = "tick",
    at = now,
}

local requests: {protocol.Request} = {
    email_one,
    sms_one,
    webhook_one,
    tick,
}

local store = app:new_store("delivery-1", now)
local summary_result = app:run(store, requests, now)
if not summary_result.ok then
    local message: string = summary_result.error.message
    local retryable: boolean = summary_result.error.retryable
else
    local summary = summary_result.value
    local runtime_id: string = summary.id
    local total_processed: number = summary.total_processed
    local sent_count: number = summary.sent_count
    local queued_count: number = summary.queued_count
    local retrying_count: number = summary.retrying_count
    local elapsed_seconds: time.Duration = summary.elapsed_seconds
    local last_status: string? = summary.last_status
end

local label_result = result.map(summary_result, function(summary: protocol.RunSummary): string
    return summary.id .. ":" .. tostring(summary.sent_count + summary.retrying_count)
end)

if label_result.ok then
    local label: string = label_result.value
end

local checked_result = result.and_then(summary_result, function(summary: protocol.RunSummary): StringResult
    if summary.retrying_count == 0 then
        return {
            ok = false,
            error = {
                code = "invalid",
                message = "expected retry",
                retryable = false,
            },
        }
    end
    return {ok = true, value = summary.id}
end)

if checked_result.ok then
    local checked_id: string = checked_result.value
end

local record_one = store:lookup_record("msg-1")
if record_one then
    local tenant_id: string = record_one.tenant_id
    local source: string? = record_one.source
    local attempts = record_one.attempts["email"]
    if attempts then
        local email_attempts: integer = attempts
    end
    local receipt = record_one.last_receipt
    if receipt then
        local provider: string = receipt.provider_id
    end
end

local cached = store:lookup_receipt("webhook:msg-3")
if cached then
    local provider_id: string = cached.provider_id
    local retry_at = cached.retry_after
    if retry_at then
        local retry_seconds: integer = retry_at:unix()
    end
end

local sent_counter = store.state.counters["sent"]
if sent_counter then
    local sent_value: integer = sent_counter
end

local queue_counter = store.state.counters["queued"]
if queue_counter then
    local queued_value: integer = queue_counter
end

local retry_counter = store.state.counters["retrying"]
if retry_counter then
    local retrying_value: integer = retry_counter
end

local last_seen = store.state.last_delivery_at
if last_seen then
    local last_seconds: integer = last_seen:unix()
end

if last_runtime_id then
    local runtime_id: string = last_runtime_id
end
