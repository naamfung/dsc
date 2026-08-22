local time = require("time")
local protocol = require("protocol")
local helpers = require("helpers")

type DeliveryStore = {
    state: protocol.StoreState,
    lookup_record: (self: DeliveryStore, message_id: string) -> protocol.DeliveryRecord?,
    lookup_receipt: (self: DeliveryStore, provider_id: string) -> protocol.DeliveryReceipt?,
    push_step: (self: DeliveryStore, step: protocol.DeliveryStep, at: time.Time) -> (),
    record_receipt: (self: DeliveryStore, request: protocol.Request, receipt: protocol.DeliveryReceipt) -> (),
    summarize: (self: DeliveryStore, now: time.Time, last_status: string?) -> protocol.RunSummary,
}

type Store = DeliveryStore

local Store = {}
Store.__index = Store

local M = {}
M.DeliveryStore = DeliveryStore

function M.new(id: string, now: time.Time): DeliveryStore
    local self: Store = {
        state = {
            id = id,
            started_at = now,
            last_delivery_at = nil,
            records = {},
            cached_receipts = {},
            steps = {},
            counters = {},
            flags = {},
        },
        lookup_record = Store.lookup_record,
        lookup_receipt = Store.lookup_receipt,
        push_step = Store.push_step,
        record_receipt = Store.record_receipt,
        summarize = Store.summarize,
    }
    setmetatable(self, Store)
    return self
end

function Store:lookup_record(message_id: string): protocol.DeliveryRecord?
    return self.state.records[message_id]
end

function Store:lookup_receipt(provider_id: string): protocol.DeliveryReceipt?
    return self.state.cached_receipts[provider_id]
end

function Store:push_step(step: protocol.DeliveryStep, at: time.Time)
    table.insert(self.state.steps, step)
    self.state.last_delivery_at = at
end

function Store:record_receipt(request: protocol.Request, receipt: protocol.DeliveryReceipt)
    if request.kind == "tick" then
        return
    end

    local source = helpers.tag_value(request, "source")
    local priority = helpers.tag_value(request, "priority")

    local attempts: {[string]: integer} = {}
    local current = self.state.records[request.message_id]
    if current then
        attempts = current.attempts
    end

    local existing = attempts[receipt.channel]
    if existing then
        attempts[receipt.channel] = existing + 1
    else
        attempts[receipt.channel] = 1
    end

    local record: protocol.DeliveryRecord = {
        tenant_id = request.tenant_id,
        message_id = request.message_id,
        channel = receipt.channel,
        last_status = receipt.local_status,
        attempts = attempts,
        rendered_preview = receipt.preview,
        last_receipt = receipt,
        updated_at = receipt.delivered_at,
        source = source,
        priority = priority,
        last_error = nil,
    }

    self.state.records[request.message_id] = record
    self.state.cached_receipts[receipt.provider_id] = receipt

    if receipt.counter_key then
        helpers.bump_counter(self.state.counters, receipt.counter_key)
    end
    helpers.bump_counter(self.state.counters, receipt.local_status)
end

function Store:summarize(now: time.Time, last_status: string?): protocol.RunSummary
    local sent_count = self.state.counters["sent"] or 0
    local queued_count = self.state.counters["queued"] or 0
    local retrying_count = self.state.counters["retrying"] or 0

    return {
        id = self.state.id,
        total_processed = #self.state.steps,
        sent_count = sent_count,
        queued_count = queued_count,
        retrying_count = retrying_count,
        elapsed_seconds = now:sub(self.state.started_at),
        last_status = last_status,
    }
end

return M
