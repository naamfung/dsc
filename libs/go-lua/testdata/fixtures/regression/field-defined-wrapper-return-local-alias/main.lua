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

M.dep = {
    get = function()
        return { answer = "ok" }
    end,
}

local f: fun(): Res = M.run
local res = f()
local answer: string = res.answer
return answer
