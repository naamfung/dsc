local protocol = require("protocol")
local helpers = require("helpers")

type Decorator = (string, protocol.BusState, protocol.Event) -> string

type SubscriberBuilder = {
    name: string,
    prefix: string,
    required_tag: string?,
    flag_name: string?,
    decorator: Decorator?,
    named: (self: SubscriberBuilder, name: string) -> SubscriberBuilder,
    prefix_with: (self: SubscriberBuilder, prefix: string) -> SubscriberBuilder,
    require_tag: (self: SubscriberBuilder, tag_name: string) -> SubscriberBuilder,
    remember_flag: (self: SubscriberBuilder, flag_name: string) -> SubscriberBuilder,
    decorate: (self: SubscriberBuilder, decorator: Decorator) -> SubscriberBuilder,
    build: (self: SubscriberBuilder) -> protocol.Subscriber,
}

type Builder = SubscriberBuilder

local Builder = {}
Builder.__index = Builder

local M = {}
M.SubscriberBuilder = SubscriberBuilder

function M.new(): SubscriberBuilder
    local self: Builder = {
        name = "subscriber",
        prefix = "sub",
        required_tag = nil,
        flag_name = nil,
        decorator = nil,
        named = Builder.named,
        prefix_with = Builder.prefix_with,
        require_tag = Builder.require_tag,
        remember_flag = Builder.remember_flag,
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

function Builder:require_tag(tag_name: string): Builder
    self.required_tag = tag_name
    return self
end

function Builder:remember_flag(flag_name: string): Builder
    self.flag_name = flag_name
    return self
end

function Builder:decorate(decorator: Decorator): Builder
    self.decorator = decorator
    return self
end

function Builder:build(): protocol.Subscriber
    local name = self.name
    local prefix = self.prefix
    local required_tag = self.required_tag
    local flag_name = self.flag_name
    local decorator = self.decorator

    return function(state: protocol.BusState, event: protocol.Event): protocol.SubscriberResult
        if event.kind == "tick" then
            return {ok = true, value = nil}
        end

        local note = prefix .. ":" .. name .. ":" .. helpers.event_label(event)

        if required_tag then
            local tags = event.meta.tags
            if not tags then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = name .. " missing tags",
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
                        message = name .. " missing tag: " .. required_tag,
                        retryable = false,
                    },
                }
            end
            note = note .. ":" .. value
        end

        if flag_name then
            state.flags[flag_name] = true
        end
        if decorator then
            note = decorator(note, state, event)
        end

        return {ok = true, value = note}
    end
end

return M
