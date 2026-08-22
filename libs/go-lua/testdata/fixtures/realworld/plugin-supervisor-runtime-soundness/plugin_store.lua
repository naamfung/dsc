local time = require("time")
local protocol = require("protocol")
local helpers = require("helpers")

type PluginStore = {
    state: protocol.StoreState,
    cache_receipt: (self: PluginStore, request: protocol.DispatchRequest, receipt: protocol.OutputReceipt, at: time.Time) -> (),
    lookup_cached: (self: PluginStore, key: string) -> protocol.CachedReceipt?,
    push_step: (self: PluginStore, step: protocol.RuntimeStep, at: time.Time) -> (),
    set_flag: (self: PluginStore, flag: string) -> (),
    summarize: (self: PluginStore, now: time.Time, last_output_kind: string?) -> protocol.RunSummary,
}

type Store = PluginStore

local Store = {}
Store.__index = Store

local M = {}
M.PluginStore = PluginStore

function M.new(id: string, now: time.Time): PluginStore
    local self: Store = {
        state = {
            id = id,
            started_at = now,
            last_tick = nil,
            last_dispatch_at = nil,
            cached_receipts = {},
            plugin_counts = {},
            flags = {},
            audit_tags = {},
            steps = {},
        },
        cache_receipt = Store.cache_receipt,
        lookup_cached = Store.lookup_cached,
        push_step = Store.push_step,
        set_flag = Store.set_flag,
        summarize = Store.summarize,
    }
    setmetatable(self, Store)
    return self
end

function Store:cache_receipt(
    request: protocol.DispatchRequest,
    receipt: protocol.OutputReceipt,
    at: time.Time
)
    self.state.cached_receipts[helpers.cache_key(request)] = {
        value = receipt,
        seen_at = at,
    }
    self.state.last_dispatch_at = at
end

function Store:lookup_cached(key: string): protocol.CachedReceipt?
    return self.state.cached_receipts[key]
end

function Store:push_step(step: protocol.RuntimeStep, at: time.Time)
    table.insert(self.state.steps, step)
    self.state.last_dispatch_at = at

    if step.kind == "dispatch" then
        local current = self.state.plugin_counts[step.plugin] or 0
        self.state.plugin_counts[step.plugin] = current + 1
    elseif step.kind == "cached" then
        local current = self.state.plugin_counts["cached"] or 0
        self.state.plugin_counts["cached"] = current + 1
    elseif step.kind == "fallback" then
        local current = self.state.plugin_counts["fallback"] or 0
        self.state.plugin_counts["fallback"] = current + 1
        self.state.audit_tags["last"] = step.note
    else
        self.state.audit_tags["last"] = step.note
    end
end

function Store:set_flag(flag: string)
    self.state.flags[flag] = true
end

function Store:summarize(now: time.Time, last_output_kind: string?): protocol.RunSummary
    local cached_hits = self.state.plugin_counts["cached"] or 0
    local fallback_count = self.state.plugin_counts["fallback"] or 0

    return {
        id = self.state.id,
        processed = #self.state.steps,
        cached_hits = cached_hits,
        fallback_count = fallback_count,
        elapsed_seconds = now:sub(self.state.started_at),
        last_output_kind = last_output_kind,
    }
end

return M
