local function outer()
    type LocalType = {x: number}
    local function inner()
        local v: LocalType = {x = 1}
    end
end
