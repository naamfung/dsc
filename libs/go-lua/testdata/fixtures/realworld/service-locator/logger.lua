type LogLevel = "debug" | "info" | "warn" | "error"

type Logger = {
    level: LogLevel,
    log: (self: Logger, level: LogLevel, msg: string) -> (),
    debug: (self: Logger, msg: string) -> (),
    info: (self: Logger, msg: string) -> (),
    warn: (self: Logger, msg: string) -> (),
    error: (self: Logger, msg: string) -> (),
}

local M = {}
M.LogLevel = LogLevel
M.Logger = Logger

function M.new(level: LogLevel?): Logger
    local logger: Logger = {
        level = level or "info",
        log = function(self: Logger, level: LogLevel, msg: string)
            print("[" .. level .. "] " .. msg)
        end,
        debug = function(self: Logger, msg: string)
            self:log("debug", msg)
        end,
        info = function(self: Logger, msg: string)
            self:log("info", msg)
        end,
        warn = function(self: Logger, msg: string)
            self:log("warn", msg)
        end,
        error = function(self: Logger, msg: string)
            self:log("error", msg)
        end,
    }
    return logger
end

return M
