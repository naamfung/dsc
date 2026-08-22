local protocol = require("protocol")

type ValidatorBuilder = {
    name: string,
    required_tag: string?,
    flag_name: string?,
    named: (self: ValidatorBuilder, name: string) -> ValidatorBuilder,
    require_tag: (self: ValidatorBuilder, tag_name: string) -> ValidatorBuilder,
    remember_flag: (self: ValidatorBuilder, flag_name: string) -> ValidatorBuilder,
    build: (self: ValidatorBuilder) -> protocol.ActionValidator,
}

type Builder = ValidatorBuilder

local Builder = {}
Builder.__index = Builder

local M = {}
M.ValidatorBuilder = ValidatorBuilder

function M.new(): ValidatorBuilder
    local self: Builder = {
        name = "validator",
        required_tag = nil,
        flag_name = nil,
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

function Builder:require_tag(tag_name: string): Builder
    self.required_tag = tag_name
    return self
end

function Builder:remember_flag(flag_name: string): Builder
    self.flag_name = flag_name
    return self
end

function Builder:build(): protocol.ActionValidator
    local name = self.name
    local required_tag = self.required_tag
    local flag_name = self.flag_name

    return function(state: protocol.StoreState, action: protocol.Action): protocol.ValidationResult
        if action.kind == "tick" then
            return {ok = true, value = action}
        end

        if required_tag then
            local tags = action.meta.tags
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
        end

        if flag_name then
            state.flags[flag_name] = true
        end

        return {ok = true, value = action}
    end
end

return M
