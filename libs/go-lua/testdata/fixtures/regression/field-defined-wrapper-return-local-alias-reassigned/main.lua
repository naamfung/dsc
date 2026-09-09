type Res = { answer: string }

local M = {
    dep = {
        get = function()
            return nil
        end,
    },
}

function M.run()
    return M.dep.get()
end

M.run = function()
    return nil
end

local f: fun(): Res = M.run
return f
