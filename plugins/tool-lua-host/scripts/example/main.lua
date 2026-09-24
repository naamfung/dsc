-- example：tool-lua-host 演示插件
-- 演示 dsc 内建：llm.chat / tool.call / notify.emit / store / hook / job
-- 带 go-lua 类型注解：加载前经类型检查器静态校验
--
-- 工具名：脚本里注册的是**短名**（summarize/ping/async），宿主加载时统一加脚本名前缀
-- 再暴露给模型（本脚本目录名为 example，故模型看到 example_summarize/example_ping/
-- example_async）。故脚本内引用自身工具名时用 dsc.script.name 拼前缀，别写死。

type SummaryArgs = { prompt: string }
type PingArgs = { note: string? }

-- 本脚本工具名的前缀（= 脚本名 + "_"）：工具名要带来源，脚本引用自身时按此拼接。
local prefix: string = dsc.script.name .. "_"

-- ==================== dsc.llm.chat（宿主聚合 LLM） ====================

local function summarize(args: SummaryArgs): string
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

-- ==================== dsc.tool.call + dsc.notify.emit + dsc.store ====================

-- ping 计数存 store（演示进程内 KV 共享）
local function ping(args: PingArgs): string
    local n = dsc.store.get("ping_count") or 0
    dsc.store.set("ping_count", n + 1)
    local out: string = dsc.tool.call("shell", { command = "echo lua-host-ping" })
    local note: string = args.note or "none"
    dsc.notify.emit("lua/example", { task = "ping", note = note, count = n + 1 })
    return "note=" .. note .. " count=" .. tostring(n + 1) .. " shell=" .. out
end

dsc.register_tool("ping", {
    description = "经宿主调用 shell 工具并发布事件（演示 dsc.tool.call / dsc.notify.emit / dsc.store）",
    parameters = {
        type = "object",
        properties = { note = { type = "string", description = "备注（可被 before_tool 钩子改写）" } }
    }
}, ping)

-- ==================== dsc.job（后台任务） ====================

dsc.register_tool("async", {
    description = "后台执行一个 LUA 任务并返回任务 id（演示 dsc.job.spawn）",
    parameters = { type = "object", properties = {} }
}, function()
    local job = dsc.job.spawn(function()
        local r: string = dsc.llm.chat({ user = "用一句话介绍你自己。", max_tokens = 50 })
        return r
    end)
    return "job " .. job .. " 已启动，用 dsc.job.status 查询（本工具演示）"
end)

-- ==================== dsc.hook（脚本注册宿主钩子） ====================

-- before_tool：改写本脚本 ping 工具的入参（其余工具不动）。
-- 宿主传进来的是**模型可见的全名**（带脚本名前缀），故用 prefix 拼。
dsc.hook.before_tool(function(name: string, args: any): any
    if name == prefix .. "ping" then
        return false, "", { note = "hooked" }
    end
    return false, "", nil
end)

-- after_tool：给本脚本 ping 工具的结果加后缀
dsc.hook.after_tool(function(name: string, args: any, result: string, err: string): any
    if name == prefix .. "ping" then
        return result .. " (after-hook)", err
    end
    return result, err
end)

-- on_event：订阅宿主事件（空实现，演示注册）
dsc.hook.on_event(function(name: string, data: any) end)
