-- lib：存于同一个 .dsp 内的另一个脚本 blob，经 dsc.dsp.require 加载。
-- 演示「一个 .dsp 承载多份代码」——模块与入口同处一库，无需外部文件。
local M = {}

function M.greet(who: string): string
    return "你好，" .. who
end

return M
