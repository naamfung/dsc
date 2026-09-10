type A = {kind: "a", get_a: (self: A) -> string}
type B = {kind: "b", get_b: (self: B) -> number}
type AB = A | B

local function process(x: AB): string
    if x.kind == "a" then
        return x:get_a()
    end
    return tostring(x:get_b())
end
