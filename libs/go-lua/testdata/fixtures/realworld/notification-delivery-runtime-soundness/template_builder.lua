local result = require("result")
local protocol = require("protocol")

type TemplateDecorator = (string, protocol.Request) -> string

type TemplateBuilder = {
    name: string,
    prefix: string?,
    required_tag: string?,
    suffix: string?,
    decorator: TemplateDecorator?,
    named: (self: TemplateBuilder, name: string) -> TemplateBuilder,
    prefix_with: (self: TemplateBuilder, prefix: string) -> TemplateBuilder,
    require_tag: (self: TemplateBuilder, tag: string) -> TemplateBuilder,
    suffix_with: (self: TemplateBuilder, suffix: string) -> TemplateBuilder,
    decorate: (self: TemplateBuilder, decorator: TemplateDecorator) -> TemplateBuilder,
    build: (self: TemplateBuilder) -> protocol.TemplateRenderer,
}

type Builder = TemplateBuilder

local Builder = {}
Builder.__index = Builder

local M = {}

function M.new(): TemplateBuilder
    local self: Builder = {
        name = "template",
        prefix = nil,
        required_tag = nil,
        suffix = nil,
        decorator = nil,
        named = Builder.named,
        prefix_with = Builder.prefix_with,
        require_tag = Builder.require_tag,
        suffix_with = Builder.suffix_with,
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

function Builder:require_tag(tag: string): Builder
    self.required_tag = tag
    return self
end

function Builder:suffix_with(suffix: string): Builder
    self.suffix = suffix
    return self
end

function Builder:decorate(decorator: TemplateDecorator): Builder
    self.decorator = decorator
    return self
end

function Builder:build(): protocol.TemplateRenderer
    local name = self.name
    local prefix = self.prefix
    local required_tag = self.required_tag
    local suffix = self.suffix
    local decorator = self.decorator

    return function(_state: protocol.StoreState, request: protocol.Request): protocol.TemplateResult
        if request.kind == "tick" then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = name .. " cannot render ticks",
                    retryable = false,
                },
            }
        end

        if required_tag then
            local tags = request.meta.tags
            if not tags or not tags[required_tag] then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = name .. " missing tag: " .. required_tag,
                        retryable = false,
                    },
                }
            end
        end

        local body: string
        if request.kind == "email" then
            body = request.subject .. ":" .. request.recipient
        elseif request.kind == "sms" then
            body = request.phone .. ":" .. request.template
        else
            body = request.endpoint .. ":" .. request.template
        end

        if prefix then
            body = prefix .. ":" .. body
        end
        if suffix then
            body = body .. ":" .. suffix
        end
        if decorator then
            body = decorator(body, request)
        end

        return {ok = true, value = body}
    end
end

return M
