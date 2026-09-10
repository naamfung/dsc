local M = {
	dep = {
		get = function()
			return nil
		end,
	},
}

M.dep = {
	get = function()
		return { answer = "ok" }
	end,
}

local res = M.dep.get()
local answer: string = res.answer
return answer
