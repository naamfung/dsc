local protocol = require("protocol")

type ValidatorBuilder = {
    name: string,
    required_tag: string?,
    required_scope: string?,
    required_resource_prefix: string?,
    flag_name: string?,
    named: (self: ValidatorBuilder, name: string) -> ValidatorBuilder,
    require_tag: (self: ValidatorBuilder, tag: string) -> ValidatorBuilder,
    require_scope: (self: ValidatorBuilder, scope: string) -> ValidatorBuilder,
    require_resource_prefix: (self: ValidatorBuilder, prefix: string) -> ValidatorBuilder,
    remember_flag: (self: ValidatorBuilder, flag_name: string) -> ValidatorBuilder,
    build: (self: ValidatorBuilder) -> protocol.RequestValidator,
}

type Builder = ValidatorBuilder

local Builder = {}
Builder.__index = Builder

local M = {}

function M.new(): ValidatorBuilder
    local self: Builder = {
        name = "validator",
        required_tag = nil,
        required_scope = nil,
        required_resource_prefix = nil,
        flag_name = nil,
        named = Builder.named,
        require_tag = Builder.require_tag,
        require_scope = Builder.require_scope,
        require_resource_prefix = Builder.require_resource_prefix,
        remember_flag = Builder.remember_flag,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:named(name: string): Builder
    self.name = name
    return self
end

function Builder:require_tag(tag: string): Builder
    self.required_tag = tag
    return self
end

function Builder:require_scope(scope: string): Builder
    self.required_scope = scope
    return self
end

function Builder:require_resource_prefix(prefix: string): Builder
    self.required_resource_prefix = prefix
    return self
end

function Builder:remember_flag(flag_name: string): Builder
    self.flag_name = flag_name
    return self
end

function Builder:build(): protocol.RequestValidator
    local name = self.name
    local required_tag = self.required_tag
    local required_scope = self.required_scope
    local required_resource_prefix = self.required_resource_prefix
    local flag_name = self.flag_name

    return function(state: protocol.StoreState, request: protocol.Request): protocol.ValidationResult
        if request.kind == "tick" then
            return {ok = true, value = request}
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

        if required_scope and request.kind == "auth" and request.scope ~= required_scope then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = name .. " wrong scope: " .. request.scope,
                    retryable = false,
                },
            }
        end

        if required_resource_prefix and request.kind ~= "auth" then
            local resource: string
            if request.kind == "query" then
                resource = request.resource
            else
                resource = request.resource
            end
            if string.sub(resource, 1, #required_resource_prefix) ~= required_resource_prefix then
                return {
                    ok = false,
                    error = {
                        code = "invalid",
                        message = name .. " wrong resource prefix",
                        retryable = false,
                    },
                }
            end
        end

        if flag_name then
            state.flags[flag_name] = true
        end

        return {ok = true, value = request}
    end
end

return M
