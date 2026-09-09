type A = {tag: "a", value: string}
type B = {tag: "b", value: number}
local r: A | B = {tag="a", value="x"}

if r.tag == "a" then
    local s: string = r.value
else
    local n: number = r.value
end
