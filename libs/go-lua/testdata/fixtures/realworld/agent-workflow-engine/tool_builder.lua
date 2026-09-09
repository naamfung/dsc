local protocol = require("protocol")

type ToolResultResult = protocol.ToolResultResult
type Formatter = (string, protocol.SessionState, protocol.ToolCallMessage) -> string

type ToolBuilder = {
    name: string,
    required_arg: string,
    prefix: string,
    mark_flag: string?,
    formatter: Formatter?,
    named: (self: ToolBuilder, name: string) -> ToolBuilder,
    require_arg: (self: ToolBuilder, key: string) -> ToolBuilder,
    prefix_with: (self: ToolBuilder, prefix: string) -> ToolBuilder,
    remember_flag: (self: ToolBuilder, flag: string) -> ToolBuilder,
    with_formatter: (self: ToolBuilder, formatter: Formatter) -> ToolBuilder,
    build: (self: ToolBuilder) -> protocol.ToolHandler,
}

type Builder = ToolBuilder

local Builder = {}
Builder.__index = Builder

local M = {}
M.ToolBuilder = ToolBuilder

function M.new(): ToolBuilder
    local self: Builder = {
        name = "tool",
        required_arg = "value",
        prefix = "tool",
        mark_flag = nil,
        formatter = nil,
        named = Builder.named,
        require_arg = Builder.require_arg,
        prefix_with = Builder.prefix_with,
        remember_flag = Builder.remember_flag,
        with_formatter = Builder.with_formatter,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:named(name: string): Builder
    self.name = name
    return self
end

function Builder:require_arg(key: string): Builder
    self.required_arg = key
    return self
end

function Builder:prefix_with(prefix: string): Builder
    self.prefix = prefix
    return self
end

function Builder:remember_flag(flag: string): Builder
    self.mark_flag = flag
    return self
end

function Builder:with_formatter(formatter: Formatter): Builder
    self.formatter = formatter
    return self
end

function Builder:build(): protocol.ToolHandler
    local name = self.name
    local required_arg = self.required_arg
    local prefix = self.prefix
    local mark_flag = self.mark_flag
    local formatter = self.formatter

    return function(state: protocol.SessionState, msg: protocol.ToolCallMessage): ToolResultResult
        local value = msg.arguments[required_arg]
        if type(value) ~= "string" then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = name .. " " .. required_arg .. " must be string",
                    retryable = false,
                },
            }
        end

        local content = prefix .. ":" .. value
        if formatter then
            content = formatter(content, state, msg)
        end

        if mark_flag and state.flags[mark_flag] then
            content = content .. ":flagged"
        end

        return {
            ok = true,
            value = {
                tool = msg.tool,
                content = content,
                cached = false,
            },
        }
    end
end

return M
