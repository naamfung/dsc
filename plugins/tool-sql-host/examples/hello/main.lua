-- hello：tool-sql-host 示例 .dsp 插件。
--
-- 一个 .dsp 文件同时是「程序」与「数据」：代码（dsp_blobs）、元数据（dsp_meta）、
-- 运行期状态（dsp_state）与自有表（schema.sql 建立）都在同一个 SQLite 库里。
-- 加载它的是 tool-sql-host 自身（无需文件可执行位），插件读写的数据就是自己那个文件。
--
-- 演示：dsc.store.*（自持 KV，重启宿主后计数仍在）/ dsc.sql.*（自有表读写）/
--       dsc.dsp.require（同一 .dsp 内的另一个脚本 blob）/ dsc.notify.emit（宿主事件）
--
-- 工具名：脚本里注册的是**短名**（greet/note/stats），宿主加载时统一加插件名前缀再
-- 暴露给模型（本插件名 hello，故模型看到 hello_greet/hello_note/hello_stats）。前缀
-- 由宿主按 dsc.QualifyToolName 合成，脚本不必也不该自己写前缀。

type HelloArgs = { name: string? }
type NoteArgs = { text: string }

local lib = dsc.dsp.require("lib")

-- ==================== 自持状态（dsc.store） ====================

-- 调用计数写在插件自身 .dsp 的 dsp_state 里，随文件一起落盘、重启不丢。
local function greet(args: HelloArgs): string
    local n = dsc.store.get("hello_count")
    if n == nil then
        n = 0
    end
    n = n + 1
    dsc.store.set("hello_count", n)

    local who = args.name
    if who == nil then
        who = "world"
    end
    dsc.notify.emit("dsp/hello", { who = who, count = n })
    return lib.greet(who) .. "（第 " .. tostring(n) .. " 次调用；插件 " .. dsc.plugin.name .. "）"
end

dsc.register_tool("greet", {
    description = "打招呼，并把调用次数记在插件自身的 .dsp 状态里（演示 dsc.store 自持状态）",
    parameters = {
        type = "object",
        properties = { name = { type = "string", description = "称呼（缺省 world）" } }
    }
}, greet)

-- ==================== 自身库 SQL（dsc.sql） ====================

-- note 表由打包期 schema.sql 建立；运行期只做数据读写，不做 DDL。
local function note(args: NoteArgs): string
    dsc.sql.exec("INSERT INTO note(text, at) VALUES(?, datetime('now'))", { args.text })
    local rows = dsc.sql.query("SELECT id, text FROM note ORDER BY id DESC LIMIT 5")
    local lines = {}
    for i = 1, #rows do
        local r = rows[i]
        lines[#lines + 1] = tostring(r.id) .. ". " .. r.text
    end
    local total = dsc.sql.query("SELECT COUNT(*) AS n FROM note")
    return "已记录「" .. args.text .. "」，共 " .. tostring(total[1].n) .. " 条；最近：\n"
        .. table.concat(lines, "\n")
end

dsc.register_tool("note", {
    description = "在插件自身 .dsp 的自有表 note 里追加一条记录并返回最近若干条（演示 dsc.sql 自持数据）",
    parameters = {
        type = "object",
        properties = { text = { type = "string", description = "记录内容" } },
        required = { "text" }
    }
}, note)

-- ==================== 自省（dsc.plugin / dsc.sql.tables） ====================

dsc.register_tool("stats", {
    description = "查看本插件自身的载体信息与库内表（演示 dsc.plugin 自省与 dsc.sql.tables）",
    parameters = { type = "object", properties = {} }
}, function(): string
    local tables = dsc.sql.tables()
    local ro = "否"
    if dsc.plugin.readonly then
        ro = "是"
    end
    return "插件=" .. dsc.plugin.name
        .. " 语言=lua"
        .. " 只读=" .. ro
        .. " 载体=" .. dsc.plugin.path
        .. " 表=" .. table.concat(tables, ",")
end)
