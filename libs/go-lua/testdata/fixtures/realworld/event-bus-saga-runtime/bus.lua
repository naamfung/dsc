local time = require("time")
local protocol = require("protocol")
local helpers = require("helpers")
local checkpoint_store = require("checkpoint_store")

type EventBus = {
    subscribers: {[string]: {protocol.Subscriber}},
    projectors: {[string]: {protocol.Projector}},
    hooks: {protocol.StepHook},
    ensure_subscribers: (self: EventBus, topic: string) -> {protocol.Subscriber},
    ensure_projectors: (self: EventBus, topic: string) -> {protocol.Projector},
    register_subscriber: (self: EventBus, topic: string, subscriber: protocol.Subscriber) -> EventBus,
    register_projector: (self: EventBus, topic: string, projector: protocol.Projector) -> EventBus,
    on_step: (self: EventBus, hook: protocol.StepHook) -> EventBus,
    new_store: (self: EventBus, id: string, now: time.Time) -> checkpoint_store.CheckpointStore,
    emit: (self: EventBus, store: checkpoint_store.CheckpointStore, step: protocol.DispatchStep, at: time.Time) -> (),
    publish: (self: EventBus, store: checkpoint_store.CheckpointStore, topic: string, event: protocol.Event, at: time.Time) -> protocol.PublishResult,
    replay: (self: EventBus, store: checkpoint_store.CheckpointStore, topic: string, events: {protocol.Event}, now: time.Time) -> protocol.ReplayResult,
}

type Bus = EventBus

local Bus = {}
Bus.__index = Bus

local M = {}
M.EventBus = EventBus

function M.new(): EventBus
    local self: Bus = {
        subscribers = {},
        projectors = {},
        hooks = {},
        ensure_subscribers = Bus.ensure_subscribers,
        ensure_projectors = Bus.ensure_projectors,
        register_subscriber = Bus.register_subscriber,
        register_projector = Bus.register_projector,
        on_step = Bus.on_step,
        new_store = Bus.new_store,
        emit = Bus.emit,
        publish = Bus.publish,
        replay = Bus.replay,
    }
    setmetatable(self, Bus)
    return self
end

function Bus:ensure_subscribers(topic: string): {protocol.Subscriber}
    local current = self.subscribers[topic]
    if current then
        return current
    end

    local created: {protocol.Subscriber} = {}
    self.subscribers[topic] = created
    return created
end

function Bus:ensure_projectors(topic: string): {protocol.Projector}
    local current = self.projectors[topic]
    if current then
        return current
    end

    local created: {protocol.Projector} = {}
    self.projectors[topic] = created
    return created
end

function Bus:register_subscriber(topic: string, subscriber: protocol.Subscriber): Bus
    table.insert(self:ensure_subscribers(topic), subscriber)
    return self
end

function Bus:register_projector(topic: string, projector: protocol.Projector): Bus
    table.insert(self:ensure_projectors(topic), projector)
    return self
end

function Bus:on_step(hook: protocol.StepHook): Bus
    table.insert(self.hooks, hook)
    return self
end

function Bus:new_store(id: string, now: time.Time): checkpoint_store.CheckpointStore
    return checkpoint_store.new(id, now)
end

function Bus:emit(store: checkpoint_store.CheckpointStore, step: protocol.DispatchStep, at: time.Time)
    store:push_step(step, at)
    for _, hook in ipairs(self.hooks) do
        hook(step, store.state)
    end
end

function Bus:publish(
    store: checkpoint_store.CheckpointStore,
    topic: string,
    event: protocol.Event,
    at: time.Time
): protocol.PublishResult
    local projectors = self:ensure_projectors(topic)
    for _, projector in ipairs(projectors) do
        projector(store.state, event, at)
    end

    if event.kind == "tick" then
        local audit_step: protocol.DispatchStep = {kind = "audit", note = "tick", at = event.at}
        self:emit(store, audit_step, at)
        return {ok = true, value = nil}
    end

    local subscribers = self:ensure_subscribers(topic)
    for _, subscriber in ipairs(subscribers) do
        local note_result: protocol.SubscriberResult = subscriber(store.state, event)
        if not note_result.ok then
            return {ok = false, error = note_result.error}
        end

        local note = note_result.value
        if note then
            local subscriber_step: protocol.DispatchStep = {
                kind = "subscriber",
                topic = topic,
                note = note,
                projection_id = helpers.event_id(event),
            }
            self:emit(store, subscriber_step, at)
        end
    end

    if event.kind == "completed" then
        return {ok = true, value = "completed"}
    end
    if event.kind == "failed" then
        return {ok = true, value = "failed"}
    end
    return {ok = true, value = nil}
end

function Bus:replay(
    store: checkpoint_store.CheckpointStore,
    topic: string,
    events: {protocol.Event},
    now: time.Time
): protocol.ReplayResult
    local last_status: string? = nil

    for _, event in ipairs(events) do
        local publish_result: protocol.PublishResult = self:publish(store, topic, event, now)
        if not publish_result.ok then
            return {ok = false, error = publish_result.error}
        end
        if publish_result.value then
            last_status = publish_result.value
        end
    end

    return {ok = true, value = store:summarize(now, last_status)}
end

return M
