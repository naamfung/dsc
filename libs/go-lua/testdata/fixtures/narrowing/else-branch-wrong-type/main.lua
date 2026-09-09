type A = {tag: "a", value: string}
type B = {tag: "b", value: number}
local r: A | B = {tag="a", value="x"}

if r.tag == "a" then
else
    local s: string = r.value -- expect-error: cannot assign
end
