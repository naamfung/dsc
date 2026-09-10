type HasName = {name: string}
local function wrap<T: HasName>(x: T): T
    return x
end
local n: number = wrap(42) -- expect-error
