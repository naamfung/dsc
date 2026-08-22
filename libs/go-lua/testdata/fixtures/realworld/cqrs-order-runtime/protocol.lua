local time = require("time")
local result = require("result")

type AppError = result.AppError

type CommandMeta = {
    trace_id: string,
    tags: {[string]: string}?,
}

type CreateOrderCommand = {
    kind: "create",
    id: string,
    customer: string,
    meta: CommandMeta,
}

type ReserveItemCommand = {
    kind: "reserve",
    id: string,
    item_id: string,
    meta: CommandMeta,
}

type CompleteOrderCommand = {
    kind: "complete",
    id: string,
    meta: CommandMeta,
}

type TickCommand = {
    kind: "tick",
    at: time.Time,
}

type Command = CreateOrderCommand | ReserveItemCommand | CompleteOrderCommand | TickCommand

type OrderAggregate = {
    id: string,
    customer: string,
    version: integer,
    status: "created" | "reserved" | "completed",
    item_id: string?,
    source: string?,
    updated_at: time.Time?,
}

type OrderView = {
    id: string,
    status: "created" | "reserved" | "completed",
    version: integer,
    item_id: string?,
    source: string?,
    completed_at: time.Time?,
}

type RunStep = {
    kind: "command",
    name: string,
    note: string,
    order_id: string?,
} | {
    kind: "audit",
    note: string,
    at: time.Time,
}

type StoreState = {
    id: string,
    started_at: time.Time,
    last_command_at: time.Time?,
    steps: {RunStep},
    orders: {[string]: OrderAggregate},
    views: {[string]: OrderView},
    counters: {[string]: integer},
    flags: {[string]: boolean},
}

type RunSummary = {
    id: string,
    total_steps: number,
    order_count: number,
    completed_count: number,
    last_status: string?,
    elapsed_seconds: number,
}

type ValidationResult = {ok: true, value: Command} | {ok: false, error: AppError}
type HandlerResult = {ok: true, value: string?} | {ok: false, error: AppError}
type ExecuteResult = {ok: true, value: string?} | {ok: false, error: AppError}
type RunResult = {ok: true, value: RunSummary} | {ok: false, error: AppError}

type CommandValidator = (StoreState, Command) -> ValidationResult
type CommandHandler = (StoreState, Command, time.Time) -> HandlerResult
type StepHook = (RunStep, StoreState) -> ()

local M = {}
M.AppError = AppError
M.CommandMeta = CommandMeta
M.CreateOrderCommand = CreateOrderCommand
M.ReserveItemCommand = ReserveItemCommand
M.CompleteOrderCommand = CompleteOrderCommand
M.TickCommand = TickCommand
M.Command = Command
M.OrderAggregate = OrderAggregate
M.OrderView = OrderView
M.RunStep = RunStep
M.StoreState = StoreState
M.RunSummary = RunSummary
M.ValidationResult = ValidationResult
M.HandlerResult = HandlerResult
M.ExecuteResult = ExecuteResult
M.RunResult = RunResult
M.CommandValidator = CommandValidator
M.CommandHandler = CommandHandler
M.StepHook = StepHook

function M.meta(trace_id: string, tags: {[string]: string}?): CommandMeta
    return {
        trace_id = trace_id,
        tags = tags,
    }
end

return M
