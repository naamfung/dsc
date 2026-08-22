local time = require("time")
local result = require("result")

type AppError = result.AppError

type PolicyMeta = {
    trace_id: string,
    tags: {[string]: string}?,
}

type AuthRequest = {
    kind: "auth",
    tenant_id: string,
    actor_id: string,
    scope: string,
    meta: PolicyMeta,
}

type QueryRequest = {
    kind: "query",
    tenant_id: string,
    actor_id: string,
    resource: string,
    meta: PolicyMeta,
}

type UpdateRequest = {
    kind: "update",
    tenant_id: string,
    actor_id: string,
    resource: string,
    change_set: {[string]: string},
    meta: PolicyMeta,
}

type TickRequest = {
    kind: "tick",
    at: time.Time,
}

type Request = AuthRequest | QueryRequest | UpdateRequest | TickRequest

type AllowDecision = {
    kind: "allow",
    reason: string,
    cache_key: string?,
    expires_at: time.Time?,
}

type DenyDecision = {
    kind: "deny",
    reason: string,
    retryable: boolean,
}

type DeferDecision = {
    kind: "defer",
    queue: string,
    retry_at: time.Time,
}

type Decision = AllowDecision | DenyDecision | DeferDecision

type TenantPolicy = {
    tenant_id: string,
    scopes: {[string]: boolean},
    allowed_resources: {[string]: boolean},
    fallback_queue: string?,
    tags: {[string]: string}?,
    last_checked: time.Time?,
}

type DecisionStep = {
    kind: "decision",
    request_kind: "auth" | "query" | "update",
    tenant_id: string,
    note: string,
}

type DeferStep = {
    kind: "defer",
    tenant_id: string,
    queue: string,
    note: string,
    retry_at: time.Time,
}

type AuditStep = {
    kind: "audit",
    note: string,
    at: time.Time,
}

type PolicyStep = DecisionStep | DeferStep | AuditStep

type StoreState = {
    id: string,
    started_at: time.Time,
    last_eval_at: time.Time?,
    policies: {[string]: TenantPolicy},
    cached_decisions: {[string]: Decision},
    steps: {PolicyStep},
    counters: {[string]: integer},
    flags: {[string]: boolean},
}

type RunSummary = {
    id: string,
    total_processed: number,
    allowed_count: number,
    denied_count: number,
    deferred_count: number,
    elapsed_seconds: time.Duration,
    last_kind: string?,
}

type ValidationResult = {ok: true, value: Request} | {ok: false, error: AppError}
type EvaluateResult = {ok: true, value: Decision?} | {ok: false, error: AppError}
type RunResult = {ok: true, value: RunSummary} | {ok: false, error: AppError}

type RequestValidator = (StoreState, Request) -> ValidationResult
type PolicyEvaluator = (StoreState, Request, time.Time) -> EvaluateResult
type StepHook = (PolicyStep, StoreState) -> ()

local M = {}
M.AppError = AppError
M.PolicyMeta = PolicyMeta
M.AuthRequest = AuthRequest
M.QueryRequest = QueryRequest
M.UpdateRequest = UpdateRequest
M.TickRequest = TickRequest
M.Request = Request
M.AllowDecision = AllowDecision
M.DenyDecision = DenyDecision
M.DeferDecision = DeferDecision
M.Decision = Decision
M.TenantPolicy = TenantPolicy
M.DecisionStep = DecisionStep
M.DeferStep = DeferStep
M.AuditStep = AuditStep
M.PolicyStep = PolicyStep
M.StoreState = StoreState
M.RunSummary = RunSummary
M.ValidationResult = ValidationResult
M.EvaluateResult = EvaluateResult
M.RunResult = RunResult
M.RequestValidator = RequestValidator
M.PolicyEvaluator = PolicyEvaluator
M.StepHook = StepHook

function M.meta(trace_id: string, tags: {[string]: string}?): PolicyMeta
    return {
        trace_id = trace_id,
        tags = tags,
    }
end

return M
