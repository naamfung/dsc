type Callback = (x: number) -> string
local cb: Callback = function(x: number): string
    return tostring(x)
end
