type HasName = {name: string}
local function wrap<T: HasName>(x: T): T
    return x
end
local r = wrap({name = "Alice"})
local s: string = r.name
