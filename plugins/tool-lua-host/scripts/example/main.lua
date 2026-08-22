-- example：tool-lua-host 演示插件
-- 演示 dsc 内建三件套：dsc.llm.chat / dsc.tool.call / dsc.notify.emit
-- 带 go-lua 类型注解：加载前经类型检查器静态校验

type SummaryArgs = { prompt: string }
type PingArgs = {}

local function summarize(args)
    -- 用宿主聚合 LLM 做摘要
    local r: string = dsc.llm.chat({
        system = "你是一位简洁的摘要助手，直接输出摘要。",
        user = args.prompt,
        max_tokens = 200
    })
    return r
end

dsc.register_tool("summarize", {
    description = "用宿主 LLM 对文本做摘要（演示 dsc.llm.chat）",
    parameters = {
        type = "object",
        properties = { prompt = { type = "string", description = "待摘要文本" } },
        required = { "prompt" }
    }
}, summarize)

local function ping(args)
    -- 经宿主转发调用 shell 工具，并发布事件到宿主事件总线
    local out: string = dsc.tool.call("shell", { command = "echo lua-host-ping" })
    dsc.notify.emit("lua/example", { task = "ping", output = out })
    return out
end

dsc.register_tool("ping", {
    description = "经宿主调用 shell 工具并发布 lua/example 事件（演示 dsc.tool.call + dsc.notify.emit）",
    parameters = { type = "object", properties = {} }
}, ping)
