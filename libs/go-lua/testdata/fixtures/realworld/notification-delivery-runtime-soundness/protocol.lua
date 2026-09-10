local time = require("time")
local result = require("result")

type AppError = result.AppError

type DeliveryMeta = {
    trace_id: string,
    tags: {[string]: string}?,
}

type EmailRequest = {
    kind: "email",
    tenant_id: string,
    message_id: string,
    recipient: string,
    subject: string,
    template: string,
    meta: DeliveryMeta,
}

type SmsRequest = {
    kind: "sms",
    tenant_id: string,
    message_id: string,
    phone: string,
    template: string,
    meta: DeliveryMeta,
}

type WebhookRequest = {
    kind: "webhook",
    tenant_id: string,
    message_id: string,
    endpoint: string,
    template: string,
    meta: DeliveryMeta,
}

type TickRequest = {
    kind: "tick",
    at: time.Time,
}

type Request = EmailRequest | SmsRequest | WebhookRequest | TickRequest

type DeliveryReceipt = {
    message_id: string,
    tenant_id: string,
    channel: "email" | "sms" | "webhook",
    provider_id: string,
    preview: string,
    local_status: "sent" | "queued" | "retrying",
    delivered_at: time.Time,
    retry_after: time.Time?,
    tags: {[string]: string}?,
    counter_key: string?,
}

type DeliveryRecord = {
    tenant_id: string,
    message_id: string,
    channel: "email" | "sms" | "webhook",
    last_status: "sent" | "queued" | "retrying",
    attempts: {[string]: integer},
    rendered_preview: string?,
    last_receipt: DeliveryReceipt?,
    updated_at: time.Time?,
    source: string?,
    priority: string?,
    last_error: string?,
}

type DeliveryEventStep = {
    kind: "delivery",
    channel: "email" | "sms" | "webhook",
    message_id: string,
    note: string,
    provider_id: string,
}

type RetryStep = {
    kind: "retry",
    channel: "email" | "sms" | "webhook",
    message_id: string,
    note: string,
    retry_at: time.Time,
}

type AuditStep = {
    kind: "audit",
    note: string,
    at: time.Time,
}

type DeliveryStep = DeliveryEventStep | RetryStep | AuditStep

type StoreState = {
    id: string,
    started_at: time.Time,
    last_delivery_at: time.Time?,
    records: {[string]: DeliveryRecord},
    cached_receipts: {[string]: DeliveryReceipt},
    steps: {DeliveryStep},
    counters: {[string]: integer},
    flags: {[string]: boolean},
}

type RunSummary = {
    id: string,
    total_processed: number,
    sent_count: number,
    queued_count: number,
    retrying_count: number,
    elapsed_seconds: time.Duration,
    last_status: string?,
}

type TemplateResult = {ok: true, value: string} | {ok: false, error: AppError}
type TransportResult = {ok: true, value: DeliveryReceipt} | {ok: false, error: AppError}
type DeliverResult = {ok: true, value: string?} | {ok: false, error: AppError}
type RunResult = {ok: true, value: RunSummary} | {ok: false, error: AppError}

type TemplateRenderer = (StoreState, Request) -> TemplateResult
type TransportHandler = (StoreState, Request, time.Time) -> TransportResult
type StepHook = (DeliveryStep, StoreState) -> ()

local M = {}
M.AppError = AppError
M.DeliveryMeta = DeliveryMeta
M.EmailRequest = EmailRequest
M.SmsRequest = SmsRequest
M.WebhookRequest = WebhookRequest
M.TickRequest = TickRequest
M.Request = Request
M.DeliveryReceipt = DeliveryReceipt
M.DeliveryRecord = DeliveryRecord
M.DeliveryEventStep = DeliveryEventStep
M.RetryStep = RetryStep
M.AuditStep = AuditStep
M.DeliveryStep = DeliveryStep
M.StoreState = StoreState
M.RunSummary = RunSummary
M.TemplateResult = TemplateResult
M.TransportResult = TransportResult
M.DeliverResult = DeliverResult
M.RunResult = RunResult
M.TemplateRenderer = TemplateRenderer
M.TransportHandler = TransportHandler
M.StepHook = StepHook

function M.meta(trace_id: string, tags: {[string]: string}?): DeliveryMeta
    return {
        trace_id = trace_id,
        tags = tags,
    }
end

return M
