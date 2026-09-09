local result = require("result")
local protocol = require("protocol")

type RetryPolicy = protocol.RetryPolicy

type PolicyBuilder = {
    label: string,
    attempts: integer,
    factor: number,
    retryable_codes: {result.ErrorCode},
    backoff: (integer) -> number,
    named: (self: PolicyBuilder, label: string) -> PolicyBuilder,
    max_attempts: (self: PolicyBuilder, attempts: integer) -> PolicyBuilder,
    scale_by: (self: PolicyBuilder, factor: number) -> PolicyBuilder,
    retry_on: (self: PolicyBuilder, code: result.ErrorCode) -> PolicyBuilder,
    with_backoff: (self: PolicyBuilder, backoff: (integer) -> number) -> PolicyBuilder,
    build: (self: PolicyBuilder) -> RetryPolicy,
}

type Builder = PolicyBuilder

local Builder = {}
Builder.__index = Builder

local M = {}
M.PolicyBuilder = PolicyBuilder

function M.new(): PolicyBuilder
    local self: Builder = {
        label = "default",
        attempts = 1,
        factor = 1,
        retryable_codes = {},
        backoff = function(attempt: integer): number
            return attempt
        end,
        named = Builder.named,
        max_attempts = Builder.max_attempts,
        scale_by = Builder.scale_by,
        retry_on = Builder.retry_on,
        with_backoff = Builder.with_backoff,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:named(label: string): Builder
    self.label = label
    return self
end

function Builder:max_attempts(attempts: integer): Builder
    self.attempts = attempts
    return self
end

function Builder:scale_by(factor: number): Builder
    self.factor = factor
    return self
end

function Builder:retry_on(code: result.ErrorCode): Builder
    table.insert(self.retryable_codes, code)
    return self
end

function Builder:with_backoff(backoff: (integer) -> number): Builder
    self.backoff = backoff
    return self
end

function Builder:build(): RetryPolicy
    local label = self.label
    local attempts = self.attempts
    local factor = self.factor
    local retryable_codes = self.retryable_codes
    local backoff = self.backoff

    return {
        label = label,
        max_attempts = attempts,
        compute_delay = function(attempt: integer): number
            return backoff(attempt) * factor
        end,
        should_retry = function(err: result.AppError, attempt: integer): boolean
            if attempt >= attempts then
                return false
            end
            if err.retryable then
                return true
            end
            for _, code in ipairs(retryable_codes) do
                if code == err.code then
                    return true
                end
            end
            return false
        end,
    }
end

return M
