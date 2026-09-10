type A = {kind: "a", value_a: string}
type B = {kind: "b", value_b: number}
type AB = A | B

function f(x: AB)
    if x.kind == "a" then
        local v: string = x.value_a
    end
end
