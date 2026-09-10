local protocol = require("protocol")
local helpers = require("helpers")

type Decorator = (string, protocol.OrderAggregate, protocol.Command) -> string

type HandlerBuilder = {
    command_kind: "create" | "reserve" | "complete",
    note_prefix: string,
    counter_name: string?,
    source_key: string?,
    decorator: Decorator?,
    for_kind: (self: HandlerBuilder, kind: "create" | "reserve" | "complete") -> HandlerBuilder,
    prefix_with: (self: HandlerBuilder, prefix: string) -> HandlerBuilder,
    count_as: (self: HandlerBuilder, counter_name: string) -> HandlerBuilder,
    capture_source: (self: HandlerBuilder, source_key: string) -> HandlerBuilder,
    decorate: (self: HandlerBuilder, decorator: Decorator) -> HandlerBuilder,
    build: (self: HandlerBuilder) -> protocol.CommandHandler,
}

type Builder = HandlerBuilder

local Builder = {}
Builder.__index = Builder

local M = {}
M.HandlerBuilder = HandlerBuilder

function M.new(): HandlerBuilder
    local self: Builder = {
        command_kind = "create",
        note_prefix = "order",
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

function Builder:for_kind(kind: "create" | "reserve" | "complete"): Builder
    self.command_kind = kind
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

function Builder:build(): protocol.CommandHandler
    local command_kind = self.command_kind
    local note_prefix = self.note_prefix
    local counter_name = self.counter_name
    local source_key = self.source_key
    local decorator = self.decorator

    return function(state: protocol.StoreState, command: protocol.Command, at: time.Time): protocol.HandlerResult
        if command.kind == "tick" then
            return {ok = true, value = nil}
        end

        local aggregate: protocol.OrderAggregate

        if command_kind == "create" then
            if command.kind ~= "create" then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = "expected create",
                        retryable = false,
                    },
                }
            end

            local existing = state.orders[command.id]
            if existing then
                return {
                    ok = false,
                    error = {
                        code = "conflict",
                        message = "order exists: " .. command.id,
                        retryable = false,
                    },
                }
            end

            aggregate = {
                id = command.id,
                customer = command.customer,
                version = 1,
                status = "created",
                item_id = nil,
                source = nil,
                updated_at = at,
            }
            state.orders[command.id] = aggregate
        elseif command_kind == "reserve" then
            if command.kind ~= "reserve" then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = "expected reserve",
                        retryable = false,
                    },
                }
            end

            local existing = state.orders[command.id]
            if not existing then
                return {
                    ok = false,
                    error = {
                        code = "not_found",
                        message = "missing order: " .. command.id,
                        retryable = false,
                    },
                }
            end

            aggregate = existing
            aggregate.version = aggregate.version + 1
            aggregate.status = "reserved"
            aggregate.item_id = command.item_id
            aggregate.updated_at = at
        else
            if command.kind ~= "complete" then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = "expected complete",
                        retryable = false,
                    },
                }
            end

            local existing = state.orders[command.id]
            if not existing then
                return {
                    ok = false,
                    error = {
                        code = "not_found",
                        message = "missing order: " .. command.id,
                        retryable = false,
                    },
                }
            end

            aggregate = existing
            aggregate.version = aggregate.version + 1
            aggregate.status = "completed"
            aggregate.updated_at = at
        end

        if source_key then
            local tags = command.meta.tags
            if tags then
                local source = tags[source_key]
                if source then
                    aggregate.source = source
                end
            end
        end

        local view = state.views[aggregate.id]
        if not view then
            view = {
                id = aggregate.id,
                status = aggregate.status,
                version = aggregate.version,
                item_id = aggregate.item_id,
                source = aggregate.source,
                completed_at = nil,
            }
            state.views[aggregate.id] = view
        end

        view.status = aggregate.status
        view.version = aggregate.version
        view.item_id = aggregate.item_id
        view.source = aggregate.source
        if aggregate.status == "completed" then
            view.completed_at = at
        end

        if counter_name then
            local current = state.counters[counter_name] or 0
            state.counters[counter_name] = current + 1
        end

        local note = note_prefix .. ":" .. aggregate.id .. ":" .. aggregate.status .. ":" .. tostring(aggregate.version)
        if decorator then
            note = decorator(note, aggregate, command)
        end

        return {ok = true, value = note}
    end
end

return M
