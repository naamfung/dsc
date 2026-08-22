local context = require("context")
local pipeline = require("pipeline")

local p = pipeline.new()
    :add("auth", function(ctx: context.Context): context.Context
        return context.with(ctx, "user_id", "u123")
    end)
    :add("permissions", function(ctx: context.Context): context.Context
        local user_id = context.get(ctx, "user_id")
        return context.with(ctx, "can_read", user_id ~= nil)
    end)
    :add("defaults", function(ctx: context.Context): context.Context
        return context.merge(ctx, {
            locale = "en",
            timezone = "UTC",
        })
    end)

local base = context.with(context.empty(), "request_id", "req-001")
local result = p:run(base)

local user_id = context.get(result, "user_id")
local can_read = context.get(result, "can_read")
local locale = context.get(result, "locale")
local count: number = p:count()
