local logger = require("logger")
local cache = require("cache")

type Services = {
    logger: logger.Logger,
    cache: cache.Cache,
}

local M = {}

local _services: Services? = nil

function M.init(log_level: logger.LogLevel?): Services
    local s: Services = {
        logger = logger.new(log_level),
        cache = cache.new(),
    }
    _services = s
    return s
end

function M.get(): Services
    if not _services then
        return M.init()
    end
    return _services
end

function M.logger(): logger.Logger
    return M.get().logger
end

function M.cache(): cache.Cache
    return M.get().cache
end

return M
