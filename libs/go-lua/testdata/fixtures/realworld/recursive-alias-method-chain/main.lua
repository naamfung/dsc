type Builder = {
    f: (self: Builder) -> Builder,
    g: (self: Builder) -> number,
}

local b: Builder = {
    f = function(self: Builder): Builder
        return self
    end,
    g = function(self: Builder): number
        return 1
    end,
}

local n: number = b:f():g()
