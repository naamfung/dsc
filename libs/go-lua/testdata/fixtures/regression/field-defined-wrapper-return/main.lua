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

local res = M.run()
local answer: string = res.answer
return answer
