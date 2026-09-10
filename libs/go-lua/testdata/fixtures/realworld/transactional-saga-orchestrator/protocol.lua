local time = require("time")
local result = require("result")

type AppError = result.AppError

type ActionMeta = {
    trace_id: string,
    tags: {[string]: string}?,
}

type BeginAction = {
    kind: "begin",
    order_id: string,
    customer_id: string,
    meta: ActionMeta,
}

type ReserveAction = {
    kind: "reserve",
    order_id: string,
    sku: string,
    qty: integer,
    meta: ActionMeta,
}

type ChargeAction = {
    kind: "charge",
    order_id: string,
    cents: integer,
    meta: ActionMeta,
}

type CommitAction = {
    kind: "commit",
    order_id: string,
    meta: ActionMeta,
}

type CancelAction = {
    kind: "cancel",
    order_id: string,
    reason: string,
    meta: ActionMeta,
}

type TickAction = {
    kind: "tick",
    at: time.Time,
}

type Action = BeginAction | ReserveAction | ChargeAction | CommitAction | CancelAction | TickAction

type ReleaseInventory = {
    kind: "release",
    reservation_token: string,
}

type RefundPayment = {
    kind: "refund",
    payment_id: string,
}

type Compensation = ReleaseInventory | RefundPayment

type SagaAggregate = {
    order_id: string,
    customer_id: string,
    version: integer,
    status: "open" | "reserved" | "charged" | "committed" | "rolled_back",
    reservation_token: string?,
    payment_id: string?,
    last_error: string?,
    source: string?,
    updated_at: time.Time?,
    compensations: {Compensation},
}

type SagaView = {
    order_id: string,
    status: "open" | "reserved" | "charged" | "committed" | "rolled_back",
    version: integer,
    reservation_token: string?,
    payment_id: string?,
    source: string?,
    committed_at: time.Time?,
    rolled_back_at: time.Time?,
    last_error: string?,
}

type ActionStep = {
    kind: "action",
    name: string,
    note: string,
    order_id: string?,
}

type CompensationStep = {
    kind: "compensation",
    name: string,
    note: string,
    order_id: string?,
}

type AuditStep = {
    kind: "audit",
    note: string,
    at: time.Time,
}

type SagaStep = ActionStep | CompensationStep | AuditStep

type StoreState = {
    id: string,
    started_at: time.Time,
    last_action_at: time.Time?,
    steps: {SagaStep},
    sagas: {[string]: SagaAggregate},
    views: {[string]: SagaView},
    counters: {[string]: integer},
    flags: {[string]: boolean},
}

type RunSummary = {
    id: string,
    total_steps: number,
    saga_count: number,
    committed_count: number,
    rolled_back_count: number,
    last_status: string?,
    elapsed_seconds: number,
}

type ValidationResult = {ok: true, value: Action} | {ok: false, error: AppError}
type HandlerResult = {ok: true, value: string?} | {ok: false, error: AppError}
type ExecuteResult = {ok: true, value: string?} | {ok: false, error: AppError}
type RunResult = {ok: true, value: RunSummary} | {ok: false, error: AppError}

type ActionValidator = (StoreState, Action) -> ValidationResult
type ActionHandler = (StoreState, Action, time.Time) -> HandlerResult
type StepHook = (SagaStep, StoreState) -> ()

local M = {}
M.AppError = AppError
M.ActionMeta = ActionMeta
M.BeginAction = BeginAction
M.ReserveAction = ReserveAction
M.ChargeAction = ChargeAction
M.CommitAction = CommitAction
M.CancelAction = CancelAction
M.TickAction = TickAction
M.Action = Action
M.Compensation = Compensation
M.SagaAggregate = SagaAggregate
M.SagaView = SagaView
M.SagaStep = SagaStep
M.StoreState = StoreState
M.RunSummary = RunSummary
M.ValidationResult = ValidationResult
M.HandlerResult = HandlerResult
M.ExecuteResult = ExecuteResult
M.RunResult = RunResult
M.ActionValidator = ActionValidator
M.ActionHandler = ActionHandler
M.StepHook = StepHook

function M.meta(trace_id: string, tags: {[string]: string}?): ActionMeta
    return {
        trace_id = trace_id,
        tags = tags,
    }
end

return M
