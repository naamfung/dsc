local protocol = require("protocol")

type ValidatorBuilder = {
    name: string?,
    required_tag: string?,
    remembered_flag: string?,
    named: (self: ValidatorBuilder, name: string) -> ValidatorBuilder,
    require_tag: (self: ValidatorBuilder, key: string) -> ValidatorBuilder,
    remember_flag: (self: ValidatorBuilder, flag: string) -> ValidatorBuilder,
    build: (self: ValidatorBuilder) -> protocol.RequestValidator,
}

type Builder = ValidatorBuilder

local Builder = {}
Builder.__index = Builder

local M = {}
M.ValidatorBuilder = ValidatorBuilder

function M.new(): ValidatorBuilder
    local self: Builder = {
        name = nil,
        required_tag = nil,
        remembered_flag = nil,
        named = Builder.named,
        require_tag = Builder.require_tag,
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

function Builder:require_tag(key: string): Builder
    self.required_tag = key
    return self
end

function Builder:remember_flag(flag: string): Builder
    self.remembered_flag = flag
    return self
end

function Builder:build(): protocol.RequestValidator
    local name = self.name or "validator"
    local required_tag = self.required_tag
    local remembered_flag = self.remembered_flag

    return function(state: protocol.StoreState, request: protocol.DispatchRequest): protocol.ValidationResult
        if required_tag then
            local tags = request.envelope.meta.tags
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

        if remembered_flag then
            state.flags[remembered_flag] = true
        end

        return {ok = true, value = request}
    end
end

return M
