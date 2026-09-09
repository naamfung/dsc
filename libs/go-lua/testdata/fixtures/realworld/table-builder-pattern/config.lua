type Config = {
    host: string,
    port: number,
    debug: boolean,
    tags: {string}
}

type ConfigBuilder = {
    _config: Config,
    host: (self: ConfigBuilder, h: string) -> ConfigBuilder,
    port: (self: ConfigBuilder, p: number) -> ConfigBuilder,
    debug: (self: ConfigBuilder, d: boolean) -> ConfigBuilder,
    tag: (self: ConfigBuilder, t: string) -> ConfigBuilder,
    build: (self: ConfigBuilder) -> Config
}

local M = {}

function M.new(): ConfigBuilder
    local builder: ConfigBuilder = {
        _config = {host = "localhost", port = 8080, debug = false, tags = {}},
        host = function(self: ConfigBuilder, h: string): ConfigBuilder
            self._config.host = h
            return self
        end,
        port = function(self: ConfigBuilder, p: number): ConfigBuilder
            self._config.port = p
            return self
        end,
        debug = function(self: ConfigBuilder, d: boolean): ConfigBuilder
            self._config.debug = d
            return self
        end,
        tag = function(self: ConfigBuilder, t: string): ConfigBuilder
            table.insert(self._config.tags, t)
            return self
        end,
        build = function(self: ConfigBuilder): Config
            return self._config
        end
    }
    return builder
end

return M
