local x: string | number = 42
if type(x) == "string" then
    local n: number = x -- expect-error: cannot assign
end
