local time = require("time")
local result = require("result")
local protocol = require("protocol")

type PreviewDecorator = (string, protocol.Request) -> string

type TransportBuilder = {
    channel: "email" | "sms" | "webhook",
    renderer: protocol.TemplateRenderer?,
    counter_key: string?,
    required_tag: string?,
    preview_decorator: PreviewDecorator?,
    for_channel: (self: TransportBuilder, channel: "email" | "sms" | "webhook") -> TransportBuilder,
    use_renderer: (self: TransportBuilder, renderer: protocol.TemplateRenderer) -> TransportBuilder,
    count_as: (self: TransportBuilder, key: string) -> TransportBuilder,
    require_tag: (self: TransportBuilder, key: string) -> TransportBuilder,
    decorate_preview: (self: TransportBuilder, decorator: PreviewDecorator) -> TransportBuilder,
    build: (self: TransportBuilder) -> protocol.TransportHandler,
}

type Builder = TransportBuilder

local Builder = {}
Builder.__index = Builder

local M = {}

function M.new(): TransportBuilder
    local self: Builder = {
        channel = "email",
        renderer = nil,
        counter_key = nil,
        required_tag = nil,
        preview_decorator = nil,
        for_channel = Builder.for_channel,
        use_renderer = Builder.use_renderer,
        count_as = Builder.count_as,
        require_tag = Builder.require_tag,
        decorate_preview = Builder.decorate_preview,
        build = Builder.build,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:for_channel(channel: "email" | "sms" | "webhook"): Builder
    self.channel = channel
    return self
end

function Builder:use_renderer(renderer: protocol.TemplateRenderer): Builder
    self.renderer = renderer
    return self
end

function Builder:count_as(key: string): Builder
    self.counter_key = key
    return self
end

function Builder:require_tag(key: string): Builder
    self.required_tag = key
    return self
end

function Builder:decorate_preview(decorator: PreviewDecorator): Builder
    self.preview_decorator = decorator
    return self
end

function Builder:build(): protocol.TransportHandler
    local channel = self.channel
    local renderer = self.renderer
    local counter_key = self.counter_key
    local required_tag = self.required_tag
    local preview_decorator = self.preview_decorator

    return function(state: protocol.StoreState, request: protocol.Request, at: time.Time): protocol.TransportResult
        if request.kind == "tick" then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = channel .. " cannot deliver ticks",
                    retryable = false,
                },
            }
        end

        local active_renderer = renderer
        if not active_renderer then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = channel .. " missing renderer",
                    retryable = false,
                },
            }
        end

        if request.kind ~= channel then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = channel .. " wrong request kind",
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
                        message = channel .. " missing tag: " .. required_tag,
                        retryable = false,
                    },
                }
            end
        end

        local rendered = active_renderer(state, request)
        if not rendered.ok then
            return {ok = false, error = rendered.error}
        end

        local preview = rendered.value
        if preview_decorator then
            preview = preview_decorator(preview, request)
        end

        local status: "sent" | "queued" | "retrying" = "sent"
        local tags = request.meta.tags
        if request.kind == "webhook" then
            status = "queued"
        end
        if tags and tags["retry"] == "true" then
            status = "retrying"
        end

        local retry_after: time.Time? = nil
        if status == "retrying" then
            retry_after = at
        end

        local receipt: protocol.DeliveryReceipt = {
            message_id = request.message_id,
            tenant_id = request.tenant_id,
            channel = channel,
            provider_id = channel .. ":" .. request.message_id,
            preview = preview,
            local_status = status,
            delivered_at = at,
            retry_after = retry_after,
            tags = tags,
            counter_key = counter_key,
        }

        return {ok = true, value = receipt}
    end
end

return M
