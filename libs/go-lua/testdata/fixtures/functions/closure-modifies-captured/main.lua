local function counter()
    local count = 0
    return {
        inc = function() count = count + 1 end,
        get = function(): number return count end
    }
end
local c = counter()
c.inc()
local val: number = c.get()
