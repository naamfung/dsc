if true then
    type LocalPoint = {x: number, y: number}
end
local p: LocalPoint = {x = 1, y = 2} -- expect-error
