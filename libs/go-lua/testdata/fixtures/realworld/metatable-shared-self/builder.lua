type Builder = {
    _name: string,
    rename: (self: Builder, name: string) -> Builder,
    name: (self: Builder) -> string,
}

local Builder = {}
Builder.__index = Builder

function Builder.new(name: string): Builder
    local self: Builder = {
        _name = name,
        rename = Builder.rename,
        name = Builder.name,
    }
    setmetatable(self, Builder)
    return self
end

function Builder:rename(name: string): Builder
    self._name = name
    return self
end

function Builder:name(): string
    return self._name
end

local M = {}
M.new = Builder.new
return M
