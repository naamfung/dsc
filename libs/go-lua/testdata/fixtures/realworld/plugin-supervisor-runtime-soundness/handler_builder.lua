local time = require("time")
local protocol = require("protocol")
local result = require("result")

type LabelDecorator = (string, protocol.PayloadEnvelope, protocol.StoreState) -> string

type HandlerBuilder = {
    name: string?,
    prefix: string?,
    required_tag: string?,
    remembered_flag: string?,
    failure_tag: string?,
    failure_code: result.ErrorCode?,
    decorator: LabelDecorator?,
    named: (self: HandlerBuilder, name: string) -> HandlerBuilder,
    prefix_with: (self: HandlerBuilder, prefix: string) -> HandlerBuilder,
    require_tag: (self: HandlerBuilder, key: string) -> HandlerBuilder,
    remember_flag: (self: HandlerBuilder, flag: string) -> HandlerBuilder,
    fail_on_tag: (self: HandlerBuilder, key: string, code: result.ErrorCode) -> HandlerBuilder,
    decorate: (self: HandlerBuilder, fn: LabelDecorator) -> HandlerBuilder,
    build: (self: HandlerBuilder) -> protocol.PluginHandler,
}

type Builder = HandlerBuilder

local Builder = {}
Builder.__index = Builder

local M = {}
M.HandlerBuilder = HandlerBuilder

function M.new(): HandlerBuilder
    local self: Builder = {
        name = nil,
        prefix = nil,
        required_tag = nil,
        remembered_flag = nil,
        failure_tag = nil,
        failure_code = nil,
        decorator = nil,
        named = Builder.named,
        prefix_with = Builder.prefix_with,
        require_tag = Builder.require_tag,
        remember_flag = Builder.remember_flag,
        fail_on_tag = Builder.fail_on_tag,
        decorate = Builder.decorate,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:named(name: string): Builder
    self.name = name
    return self
end

function Builder:prefix_with(prefix: string): Builder
    self.prefix = prefix
    return self
end

function Builder:require_tag(key: string): Builder
    self.required_tag = key
    return self
end

function Builder:remember_flag(flag: string): Builder
    self.remembered_flag = flag
    return self
end

function Builder:fail_on_tag(key: string, code: result.ErrorCode): Builder
    self.failure_tag = key
    self.failure_code = code
    return self
end

function Builder:decorate(fn: LabelDecorator): Builder
    self.decorator = fn
    return self
end

function Builder:build(): protocol.PluginHandler
    local name = self.name or "plugin"
    local prefix = self.prefix or name
    local required_tag = self.required_tag
    local remembered_flag = self.remembered_flag
    local failure_tag = self.failure_tag
    local failure_code = self.failure_code
    local decorator = self.decorator

    return function(
        state: protocol.StoreState,
        envelope: protocol.PayloadEnvelope,
        at: time.Time
    ): protocol.DispatchResult
        local tags = envelope.meta.tags

        if required_tag then
            if not tags then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = name .. ": missing tags",
                        retryable = false,
                    },
                }
            end

            local value = tags[required_tag]
            if not value then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = name .. ": missing tag " .. required_tag,
                        retryable = false,
                    },
                }
            end
        end

        if failure_tag and tags then
            local value = tags[failure_tag]
            if value then
                return {
                    ok = false,
                    error = {
                        code = failure_code or "busy",
                        message = name .. ": tag requested retry",
                        retryable = true,
                    },
                }
            end
        end

        if remembered_flag then
            state.flags[remembered_flag] = true
        end

        local payload = envelope.payload
        local receipt: protocol.OutputReceipt

        if payload.kind == "render" then
            local subject = payload.values["subject"] or payload.template
            local body = prefix .. ":" .. payload.template .. ":" .. subject
            if decorator then
                body = decorator(body, envelope, state)
            end
            receipt = {
                plugin = name,
                envelope_id = envelope.id,
                output = {
                    kind = "rendered",
                    body = body,
                    label = prefix,
                },
                emitted_at = at,
                cached = false,
            }
        elseif payload.kind == "index" then
            receipt = {
                plugin = name,
                envelope_id = envelope.id,
                output = {
                    kind = "indexed",
                    count = #payload.terms,
                },
                emitted_at = at,
                cached = false,
            }
        else
            local note = prefix .. ":" .. payload.action .. ":" .. payload.actor_id
            if decorator then
                note = decorator(note, envelope, state)
            end
            receipt = {
                plugin = name,
                envelope_id = envelope.id,
                output = {
                    kind = "audited",
                    note = note,
                    retry_after = nil,
                },
                emitted_at = at,
                cached = false,
            }
        end

        return {ok = true, value = receipt}
    end
end

return M
