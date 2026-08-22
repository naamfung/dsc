local result = require("result")
local protocol = require("protocol")

type Decorator = (string, protocol.RuntimeState, protocol.PluginCall) -> string
type Tagger = (protocol.RetryPolicy, protocol.RuntimeState, protocol.PluginCall) -> {[string]: string}?

type PluginBuilder = {
    name: string,
    arg_key: string,
    prefix: string,
    remember_flag: string?,
    decorator: Decorator?,
    tagger: Tagger?,
    named: (self: PluginBuilder, name: string) -> PluginBuilder,
    arg: (self: PluginBuilder, key: string) -> PluginBuilder,
    prefix_with: (self: PluginBuilder, prefix: string) -> PluginBuilder,
    remember_when_flag: (self: PluginBuilder, flag: string) -> PluginBuilder,
    decorate: (self: PluginBuilder, decorator: Decorator) -> PluginBuilder,
    tag_with: (self: PluginBuilder, tagger: Tagger) -> PluginBuilder,
    build: (self: PluginBuilder) -> protocol.PluginHandler,
}

type Builder = PluginBuilder

local Builder = {}
Builder.__index = Builder

local M = {}
M.PluginBuilder = PluginBuilder

function M.new(): PluginBuilder
    local self: Builder = {
        name = "plugin",
        arg_key = "value",
        prefix = "plugin",
        remember_flag = nil,
        decorator = nil,
        tagger = nil,
        named = Builder.named,
        arg = Builder.arg,
        prefix_with = Builder.prefix_with,
        remember_when_flag = Builder.remember_when_flag,
        decorate = Builder.decorate,
        tag_with = Builder.tag_with,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:named(name: string): Builder
    self.name = name
    return self
end

function Builder:arg(key: string): Builder
    self.arg_key = key
    return self
end

function Builder:prefix_with(prefix: string): Builder
    self.prefix = prefix
    return self
end

function Builder:remember_when_flag(flag: string): Builder
    self.remember_flag = flag
    return self
end

function Builder:decorate(decorator: Decorator): Builder
    self.decorator = decorator
    return self
end

function Builder:tag_with(tagger: Tagger): Builder
    self.tagger = tagger
    return self
end

function Builder:build(): protocol.PluginHandler
    local name = self.name
    local arg_key = self.arg_key
    local prefix = self.prefix
    local remember_flag = self.remember_flag
    local decorator = self.decorator
    local tagger = self.tagger

    return function(state: protocol.RuntimeState, call: protocol.PluginCall, policy: protocol.RetryPolicy): protocol.PluginResult
        local raw = call.input[arg_key]
        if type(raw) ~= "string" then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = name .. " " .. arg_key .. " must be string",
                    retryable = false,
                },
            }
        end

        local content = prefix .. ":" .. raw
        if decorator then
            content = decorator(content, state, call)
        end
        if remember_flag and state.flags[remember_flag] then
            content = content .. ":flag"
        end

        local delay = policy.compute_delay(1)
        local tags = nil
        if tagger then
            tags = tagger(policy, state, call)
        end

        return {
            ok = true,
            value = {
                plugin = call.plugin,
                content = content .. ":" .. tostring(delay),
                cached = false,
                tags = tags,
            },
        }
    end
end

return M
