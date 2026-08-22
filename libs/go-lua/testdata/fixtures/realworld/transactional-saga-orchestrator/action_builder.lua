local time = require("time")
local protocol = require("protocol")
local helpers = require("helpers")

type Decorator = (string, protocol.SagaAggregate, protocol.Action) -> string

type ActionBuilder = {
    action_kind: "begin" | "reserve" | "charge" | "commit" | "cancel",
    note_prefix: string,
    counter_name: string?,
    source_key: string?,
    decorator: Decorator?,
    for_kind: (self: ActionBuilder, kind: "begin" | "reserve" | "charge" | "commit" | "cancel") -> ActionBuilder,
    prefix_with: (self: ActionBuilder, prefix: string) -> ActionBuilder,
    count_as: (self: ActionBuilder, counter_name: string) -> ActionBuilder,
    capture_source: (self: ActionBuilder, source_key: string) -> ActionBuilder,
    decorate: (self: ActionBuilder, decorator: Decorator) -> ActionBuilder,
    build: (self: ActionBuilder) -> protocol.ActionHandler,
}

type Builder = ActionBuilder

local Builder = {}
Builder.__index = Builder

local M = {}
M.ActionBuilder = ActionBuilder

function M.new(): ActionBuilder
    local self: Builder = {
        action_kind = "begin",
        note_prefix = "saga",
        counter_name = nil,
        source_key = nil,
        decorator = nil,
        for_kind = Builder.for_kind,
        prefix_with = Builder.prefix_with,
        count_as = Builder.count_as,
        capture_source = Builder.capture_source,
        decorate = Builder.decorate,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:for_kind(kind: "begin" | "reserve" | "charge" | "commit" | "cancel"): Builder
    self.action_kind = kind
    return self
end

function Builder:prefix_with(prefix: string): Builder
    self.note_prefix = prefix
    return self
end

function Builder:count_as(counter_name: string): Builder
    self.counter_name = counter_name
    return self
end

function Builder:capture_source(source_key: string): Builder
    self.source_key = source_key
    return self
end

function Builder:decorate(decorator: Decorator): Builder
    self.decorator = decorator
    return self
end

local function ensure_view(state: protocol.StoreState, saga: protocol.SagaAggregate): protocol.SagaView
    local current = state.views[saga.order_id]
    if current then
        return current
    end

    local created: protocol.SagaView = {
        order_id = saga.order_id,
        status = saga.status,
        version = saga.version,
        reservation_token = saga.reservation_token,
        payment_id = saga.payment_id,
        source = saga.source,
        committed_at = nil,
        rolled_back_at = nil,
        last_error = saga.last_error,
    }
    state.views[saga.order_id] = created
    return created
end

function Builder:build(): protocol.ActionHandler
    local action_kind = self.action_kind
    local note_prefix = self.note_prefix
    local counter_name = self.counter_name
    local source_key = self.source_key
    local decorator = self.decorator

    return function(state: protocol.StoreState, action: protocol.Action, at: time.Time): protocol.HandlerResult
        if action.kind == "tick" then
            return {ok = true, value = nil}
        end

        local saga: protocol.SagaAggregate

        if action_kind == "begin" then
            if action.kind ~= "begin" then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = "expected begin",
                        retryable = false,
                    },
                }
            end

            local existing = state.sagas[action.order_id]
            if existing then
                return {
                    ok = false,
                    error = {
                        code = "conflict",
                        message = "saga exists: " .. action.order_id,
                        retryable = false,
                    },
                }
            end

            saga = {
                order_id = action.order_id,
                customer_id = action.customer_id,
                version = 1,
                status = "open",
                reservation_token = nil,
                payment_id = nil,
                last_error = nil,
                source = nil,
                updated_at = at,
                compensations = {},
            }
            state.sagas[action.order_id] = saga
        elseif action_kind == "reserve" then
            if action.kind ~= "reserve" then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = "expected reserve",
                        retryable = false,
                    },
                }
            end

            local existing = state.sagas[action.order_id]
            if not existing then
                return {
                    ok = false,
                    error = {
                        code = "not_found",
                        message = "missing saga: " .. action.order_id,
                        retryable = false,
                    },
                }
            end

            saga = existing
            saga.version = saga.version + 1
            saga.status = "reserved"
            saga.reservation_token = "res:" .. action.sku .. ":" .. tostring(action.qty)
            table.insert(saga.compensations, {
                kind = "release",
                reservation_token = saga.reservation_token,
            })
            saga.updated_at = at
        elseif action_kind == "charge" then
            if action.kind ~= "charge" then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = "expected charge",
                        retryable = false,
                    },
                }
            end

            local existing = state.sagas[action.order_id]
            if not existing then
                return {
                    ok = false,
                    error = {
                        code = "not_found",
                        message = "missing saga: " .. action.order_id,
                        retryable = false,
                    },
                }
            end

            saga = existing
            saga.version = saga.version + 1
            saga.status = "charged"
            saga.payment_id = "pay:" .. tostring(action.cents)
            table.insert(saga.compensations, {
                kind = "refund",
                payment_id = saga.payment_id,
            })
            saga.updated_at = at
        elseif action_kind == "commit" then
            if action.kind ~= "commit" then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = "expected commit",
                        retryable = false,
                    },
                }
            end

            local existing = state.sagas[action.order_id]
            if not existing then
                return {
                    ok = false,
                    error = {
                        code = "not_found",
                        message = "missing saga: " .. action.order_id,
                        retryable = false,
                    },
                }
            end

            saga = existing
            saga.version = saga.version + 1
            saga.status = "committed"
            saga.updated_at = at
        else
            if action.kind ~= "cancel" then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = "expected cancel",
                        retryable = false,
                    },
                }
            end

            local existing = state.sagas[action.order_id]
            if not existing then
                return {
                    ok = false,
                    error = {
                        code = "not_found",
                        message = "missing saga: " .. action.order_id,
                        retryable = false,
                    },
                }
            end

            saga = existing
            saga.version = saga.version + 1
            saga.status = "rolled_back"
            saga.last_error = action.reason
            saga.updated_at = at
        end

        if source_key then
            local tags = action.meta.tags
            if tags then
                local source = tags[source_key]
                if source then
                    saga.source = source
                end
            end
        end

        local view = ensure_view(state, saga)
        view.status = saga.status
        view.version = saga.version
        view.reservation_token = saga.reservation_token
        view.payment_id = saga.payment_id
        view.source = saga.source
        view.last_error = saga.last_error
        if saga.status == "committed" then
            view.committed_at = at
        elseif saga.status == "rolled_back" then
            view.rolled_back_at = at
        end

        if counter_name then
            local current = state.counters[counter_name] or 0
            state.counters[counter_name] = current + 1
        end

        local note = note_prefix .. ":" .. saga.order_id .. ":" .. saga.status .. ":" .. tostring(saga.version)
        if decorator then
            note = decorator(note, saga, action)
        else
            note = note .. ":" .. helpers.action_label(action)
        end

        return {ok = true, value = note}
    end
end

return M
