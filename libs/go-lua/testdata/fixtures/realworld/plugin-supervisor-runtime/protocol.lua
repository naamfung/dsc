local time = require("time")
local result = require("result")

type AppError = result.AppError
type ErrorCode = result.ErrorCode

type RequestMeta = {
    trace_id: string,
    tags: {[string]: string}?,
}

type Envelope<T> = {
    id: string,
    tenant_id: string,
    payload: T,
    meta: RequestMeta,
}

type RenderPayload = {
    kind: "render",
    template: string,
    values: {[string]: string},
}

type IndexPayload = {
    kind: "index",
    document_id: string,
    terms: {string},
}

type AuditPayload = {
    kind: "audit",
    action: string,
    actor_id: string,
}

type Payload = RenderPayload | IndexPayload | AuditPayload
type PayloadEnvelope = Envelope<Payload>

type DispatchRequest = {
    kind: "dispatch",
    plugin: string,
    envelope: PayloadEnvelope,
}

type TickRequest = {
    kind: "tick",
    at: time.Time,
}

type Request = DispatchRequest | TickRequest

type RenderOutput = {
    kind: "rendered",
    body: string,
    label: string?,
}

type IndexOutput = {
    kind: "indexed",
    count: integer,
}

type AuditOutput = {
    kind: "audited",
    note: string,
    retry_after: time.Time?,
}

type Output = RenderOutput | IndexOutput | AuditOutput

type Receipt<T> = {
    plugin: string,
    envelope_id: string,
    output: T,
    emitted_at: time.Time,
    cached: boolean,
}

type Cached<T> = {
    value: T,
    seen_at: time.Time,
}

type OutputReceipt = Receipt<Output>
type CachedReceipt = Cached<OutputReceipt>

type DispatchStep = {
    kind: "dispatch",
    plugin: string,
    output: Output,
    cached: boolean,
}

type CachedStep = {
    kind: "cached",
    plugin: string,
    envelope_id: string,
    at: time.Time,
}

type FallbackStep = {
    kind: "fallback",
    plugin: string,
    queue: string,
    note: string,
    retry_at: time.Time,
}

type AuditStep = {
    kind: "audit",
    note: string,
    at: time.Time,
}

type RuntimeStep = DispatchStep | CachedStep | FallbackStep | AuditStep

type FallbackPlan = {
    queue: string,
    note: string,
    retry_at: time.Time,
}

type StoreState = {
    id: string,
    started_at: time.Time,
    last_tick: time.Time?,
    last_dispatch_at: time.Time?,
    cached_receipts: {[string]: CachedReceipt},
    plugin_counts: {[string]: integer},
    flags: {[string]: boolean},
    audit_tags: {[string]: string},
    steps: {RuntimeStep},
}

type RunSummary = {
    id: string,
    processed: number,
    cached_hits: number,
    fallback_count: number,
    elapsed_seconds: time.Duration,
    last_output_kind: string?,
}

type ValidationResult = {ok: true, value: DispatchRequest} | {ok: false, error: AppError}
type DispatchResult = {ok: true, value: OutputReceipt?} | {ok: false, error: AppError}
type FallbackResult = {ok: true, value: FallbackPlan?} | {ok: false, error: AppError}
type RunResult = {ok: true, value: RunSummary} | {ok: false, error: AppError}

type RequestValidator = (StoreState, DispatchRequest) -> ValidationResult
type PluginHandler = (StoreState, PayloadEnvelope, time.Time) -> DispatchResult
type FallbackHandler = (StoreState, DispatchRequest, AppError, time.Time) -> FallbackResult
type StepHook = (RuntimeStep, StoreState) -> ()

local M = {}
M.AppError = AppError
M.ErrorCode = ErrorCode
M.RequestMeta = RequestMeta
M.Envelope = Envelope
M.RenderPayload = RenderPayload
M.IndexPayload = IndexPayload
M.AuditPayload = AuditPayload
M.Payload = Payload
M.PayloadEnvelope = PayloadEnvelope
M.DispatchRequest = DispatchRequest
M.TickRequest = TickRequest
M.Request = Request
M.RenderOutput = RenderOutput
M.IndexOutput = IndexOutput
M.AuditOutput = AuditOutput
M.Output = Output
M.Receipt = Receipt
M.Cached = Cached
M.OutputReceipt = OutputReceipt
M.CachedReceipt = CachedReceipt
M.DispatchStep = DispatchStep
M.CachedStep = CachedStep
M.FallbackStep = FallbackStep
M.AuditStep = AuditStep
M.RuntimeStep = RuntimeStep
M.FallbackPlan = FallbackPlan
M.StoreState = StoreState
M.RunSummary = RunSummary
M.ValidationResult = ValidationResult
M.DispatchResult = DispatchResult
M.FallbackResult = FallbackResult
M.RunResult = RunResult
M.RequestValidator = RequestValidator
M.PluginHandler = PluginHandler
M.FallbackHandler = FallbackHandler
M.StepHook = StepHook

function M.meta(trace_id: string, tags: {[string]: string}?): RequestMeta
    return {
        trace_id = trace_id,
        tags = tags,
    }
end

return M
