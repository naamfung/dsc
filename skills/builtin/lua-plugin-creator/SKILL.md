---
name: lua-plugin-creator
description: 极全面的 LUA 插件创造指南——教模型经 tool-lua-host 编写、注册、验证与交付新的 LUA 工具插件（dsc API 完整参考、类型注解、热加载、最佳实践）。
---

# LUA 插件创造指南（Lua Plugin Creator）

本技能指导你（DSC 的 Agent）**自己创造 LUA 插件**：当现有工具无法满足需求时，编写一个 LUA 脚本，由 `tool-lua-host` 加载后注册为新工具，**无需修改宿主主程序，无需请求开发人员介入**。

---

## 1. 架构理解：你写的脚本如何变成工具

```
你写文件：  scripts/<脚本名>/main.lua
      │   tool-lua-host（空壳插件）每秒轮询脚本目录，发现新脚本/变更自动加载或重载
      ▼
  脚本执行：  dsc.register_tool("my_tool", spec, handler)
      │   注册名自动加前缀 → 宿主可见工具名 = lua_my_tool
      ▼
宿主调用：  Agent 或任意工具经宿主流水线调用 lua_my_tool → 转发到 tool-lua-host
      │   → 在你的脚本 VM 上执行 handler(args)，返回结果文本
```

关键特性：
- **独立 VM**：每个脚本一个独立的 LUA 状态机，脚本之间隔离，崩溃互不影响。
- **沙箱**：VM 不提供 `os`/`io` 库——脚本**无法直接读写文件系统、执行进程、访问网络**。需要这类能力时用 `dsc.tool.call` 调用宿主已有工具（如 `shell`、`read_file`）。
- **热加载**：脚本目录每 2 秒轮询一次。新增/修改 `main.lua` 后无需重启，自动生效；删除脚本目录则工具自动卸载。
- **类型检查**：脚本加载前经静态类型检查器校验（语法错误会阻止加载，类型诊断会打印警告但不阻止运行）。**给代码写类型注解，让问题在加载前暴露**。

---

## 2. 何时创造 LUA 插件（决策准则）

| 情形 | 行动 |
|---|---|
| 现有工具组合可满足（如先 `read_file` 再 `shell`） | **优先组合，不创造** |
| 需要多步骤、有状态、可复用的流程 | **创造插件**（脚本内可自由编排 dsc API） |
| 需要跨工具共享状态 | 用 `dsc.store` |
| 需要在宿主工具执行前后干预（拦截/改写参数/结果） | 用 `dsc.hook` |
| 需要后台长任务 | 用 `dsc.job` |
| 需要调用宿主 LLM 做推理/生成 | 用 `dsc.llm.chat` |

创造插件的回报：**一次编写，全宿主可用**——任何会话、任何 Agent、任何其他插件都能调用你注册的工具。

---

## 3. dsc API 完整参考

所有能力以全局 `dsc` 表暴露。参数/返回值均为 LUA 原生类型（table / string / number / boolean）。

### 3.1 工具注册

```
dsc.register_tool(name: string, spec: table, handler: function) -> nil
```

- `name`：工具名（小写、下划线分词）。对外暴露为 `lua_<name>`。
- `spec`：`{ description = string, parameters = <JSON Schema> }`。`parameters` 用 JSON Schema 描述参数（`type=object`、`properties`、`required`）。
- `handler`：`function(args: table) -> string | table | nil`。返回字符串直接作为工具结果；返回 table 会序列化为 JSON；返回 nil 结果为空。
- 重复注册同名工具会报错。

```lua
dsc.register_tool("greet", {
    description = "生成问候语",
    parameters = {
        type = "object",
        properties = { who = { type = "string", description = "问候对象" } },
        required = { "who" }
    }
}, function(args)
    return "你好，" .. args.who .. "！"
end)
-- 宿主侧工具名：lua_greet
```

### 3.2 LLM（宿主聚合，含多 provider 路由）

```
dsc.llm.chat({ system?: string, user: string, max_tokens?: number }) -> string
```

- 经宿主聚合 LLM 服务（primary → fallback 路由），与主 Agent 同源。
- **必须用流式路径**，thinking 模式下普通调用可能返回空文本；本函数已内部处理。
- `system` 可选；`max_tokens` 省略用服务端默认。

```lua
local summary: string = dsc.llm.chat({
    system = "你是一位简洁的摘要助手。",
    user = "请摘要这段文本：..." ,
    max_tokens = 200
})
```

### 3.3 调用其他工具（宿主转发）

```
dsc.tool.call(name: string, args?: table) -> string
dsc.tool.list() -> { {name, description}, ... }
```

- 经宿主完整工具流水线执行（含策略拦截、钩子、超时），**与 Agent 直接调用完全一致**。
- 适合编排：脚本作为"编排层"，把 shell / read_file / str_replace_editor 组合成复合工具。
- 参数用 table 传入（与目标工具的 JSON Schema 对应）。

```lua
local out: string = dsc.tool.call("shell", { command = "dir /b ." })
```

### 3.4 事件发布

```
dsc.notify.emit(name: string, data?: table) -> nil
```

- 发布事件到宿主事件总线：宿主内任何订阅者（TUI 唤醒、其他插件的 `on_event` 钩子）都能收到。
- `name` 建议用命名空间式：`<脚本名>/<事件>`（如 `myplugin/done`）。

```lua
dsc.notify.emit("myplugin/done", { task = "sync", ok = true })
```

### 3.5 共享存储（进程内 KV，脚本间共享）

```
dsc.store.get(key: string) -> any | nil
dsc.store.set(key: string, value: any) -> nil
dsc.store.delete(key: string) -> nil
```

- 所有脚本共享同一存储（进程生命周期内），用于跨工具/跨脚本传状态。
- 值支持 string / number / boolean / table（table 递归转换）。

```lua
local n = dsc.store.get("count") or 0
dsc.store.set("count", n + 1)
```

### 3.6 宿主钩子（拦截/改写工具执行）

```
dsc.hook.before_tool(fn)   -- fn(name, args) -> (veto, error, new_args)
dsc.hook.after_tool(fn)    -- fn(name, args, result, error) -> (new_result, new_error)
dsc.hook.on_event(fn)      -- fn(name, data)
```

- 钩子对所有宿主工具执行生效（含其他插件注册的工具）。多个脚本可各注册多个钩子，按注册顺序执行。
- `before_tool`：`veto=true` 阻止执行并返回 `error`；返回第三个值 `new_args` 改写参数。
- `after_tool`：改写结果/错误（返回两个值）。
- **注意**：当你的脚本自身工具正在执行（如 handler 内部调 `dsc.tool.call`）时，宿主回调钩子会**跳过**该脚本的钩子（避免 VM 重入死锁）。因此不要在钩子里依赖"自己工具内部调用的工具"会经过自己的 before 钩子。

```lua
-- 拦截：禁止对系统盘执行 shell
dsc.hook.before_tool(function(name, args)
    if name == "shell" and args and args.command and string.find(args.command, "format") then
        return true, "format 命令被 LUA 钩子拦截", nil
    end
    return false, "", nil
end)
```

### 3.7 后台任务

```
dsc.job.spawn(fn: function) -> job_id: string
dsc.job.status(job_id: string) -> string   -- "running" | "completed: <结果>" | "failed: <错误>"
dsc.job.list() -> { [job_id] = "running"|"completed"|"failed", ... }
```

- `fn` 在你的脚本 VM 上异步执行（与主执行串行化，不会并发冲突）。
- 适合耗时操作（如多次 `dsc.llm.chat`）或定时汇报。
- 结果/错误记录在任务状态，可用 `dsc.job.status` 查询。

```lua
local job = dsc.job.spawn(function()
    local r: string = dsc.llm.chat({ user = "总结当前进展。", max_tokens = 100 })
    dsc.notify.emit("myplugin/job_done", { result = r })
    return r
end)
```

---

## 4. 类型注解（go-lua 类型系统）

脚本经 flow-sensitive 类型检查器校验。**为变量、参数、返回值写类型注解**，让类型错误在加载前暴露（否则只作为警告打印）。

### 4.1 基础类型

```lua
local n: number = 42
local i: integer = 7
local s: string = "hello"
local b: boolean = true
local v: any = anything          -- any = 任意类型（关闭检查）
local x: nil = nil
```

### 4.2 表类型（record）

```lua
type Point = { x: number, y: number }
local p: Point = { x = 10, y = 20 }
local lookup: { [string]: number } = { a = 1 }
local arr: { number } = { 1, 2, 3 }
```

### 4.3 可选与联合

```lua
local maybe: string? = nil         -- 可空
type Exit = { kind: "exit", code: number }
type Msg  = { kind: "message", text: string }
type Event = Exit | Msg            -- 联合类型
```

### 4.4 泛型

```lua
local function first<T>(arr: { T }): T?
    return arr[1]
end
```

### 4.5 流程收窄（flow narrowing）

```lua
local function handle(e: Event)
    if e.kind == "exit" then
        print(e.code)   -- 此处 e 已收窄为 Exit
    else
        print(e.text)   -- 此处 e 已收窄为 Msg
    end
end
```

### 4.6 函数签名注解

```lua
local function summarize(args: SummaryArgs): string
    ...
end
```

---

## 5. 脚本结构规范

```
scripts/
└── <插件名>/              # 目录名 = 插件名（小写，连字符分隔）
    └── main.lua           # 唯一入口，加载时执行（定义工具/钩子/初始化）
```

- **一个脚本 = 一个插件**：聚焦一个职责域，注册若干相关工具。
- `main.lua` 顶层语句在加载时执行一次：定义函数 → `dsc.register_tool` → 可选注册钩子/初始化 store。
- 类型别名（`type X = ...`）建议放文件头部，便于阅读。

---

## 6. 创建流程（逐步执行）

1. **读本技能**（正在做）。若有疑问先 `read_skill` 再动手。
2. **规划**：确定要注册哪些工具、各自参数 Schema、用哪些 dsc 服务。创建模式下应先 `exit_plan_mode` 呈现设计再写码。
3. **写脚本**：用 `shell`/`str_replace_editor` 创建 `scripts/<插件名>/main.lua`（相对宿主工作目录）。
4. **验证语法**：脚本加载前会做语法门禁——语法错误会阻止加载并打印错误；类型诊断打印警告。**写完先自查**：`dsc.*` 调用、类型注解、字符串拼接（number 需 `tostring`）、table 字段名。
5. **等待热加载**：最多 2 秒轮询生效。新工具 `lua_<name>` 自动出现在宿主工具目录。
6. **验证工具**：在本轮或下一轮调用 `lua_<name>` 做冒烟测试；复杂工具准备多组参数（正常/边界/错误）。
7. **迭代**：改 `main.lua` → 2 秒后自动重载 → 重测。删除目录即卸载。

**验证要点**：
- 正常路径返回符合预期；
- 缺参/错参时能给出清晰错误（handler 内 `error()` 或返回错误文本）；
- 若用到 `dsc.hook`，确认钩子不影响无关工具。

---

## 7. 参数 Schema 规范

`parameters` 使用 JSON Schema（object 类型）：

```lua
parameters = {
    type = "object",
    properties = {
        prompt = { type = "string", description = "必填参数" },
        count  = { type = "integer", description = "可选参数" },
        tags   = { type = "array", items = { type = "string" } }
    },
    required = { "prompt" }
}
```

- 用 `description` 写清每个参数含义——这是调用方（LLM）理解你工具的窗口。
- 只声明真正需要的参数；`required` 只放必须项。
- 类型用 JSON Schema 类型（string/integer/number/boolean/array/object）。

---

## 8. 错误处理

- 工具内错误两种做法：
  - `error("...")`：抛 LUA 错误，宿主记录为工具执行失败。
  - `return "错误说明文本"`：作为正常结果返回，适合"软失败"。
- 建议：可恢复的失败返回文本说明；不可恢复用 `error()`。
- `dsc.tool.call` 调用的工具失败时抛错，可用 `pcall` 捕获后转为友好提示：

```lua
local ok, out = pcall(function() return dsc.tool.call("shell", { command = cmd }) end)
if not ok then
    return "执行失败: " .. tostring(out)
end
```

- 后台任务 `fn` 内部抛错会记录为 `failed: <错误>`，可用 `dsc.job.status` 查询。

---

## 9. 最佳实践

1. **类型注解写全**：参数/返回/局部变量，让加载前静态检查兜底。
2. **参数默认值**：`args.foo or "default"` 处理缺省。
3. **命名**：工具名小写下划线；事件名带脚本前缀（`<name>/<event>`）；store 键带脚本前缀避免冲突。
4. **幂等与安全**：脚本可被热重载（多次加载），初始化代码（store 预设等）要可重复执行。
5. **不要阻塞 VM**：handler 内避免超长循环；耗时的活交给 `dsc.job`。
6. **减少 LLM 调用**：能一次 `dsc.llm.chat` 完成就别多次；`max_tokens` 按需设小。
7. **善用组合**：先想"现有工具 + store/hook 能否解决"，再决定是否写新脚本。
8. **结果格式**：返回纯文本或 JSON；若返回 JSON，脚本用 `return { ... }`（自动序列化），便于下游解析。
9. **避免 `tostring` 陷阱**：number 拼接字符串需显式 `tostring(n)`。

---

## 10. 常见误区

| 误区 | 正解 |
|---|---|
| 想直接读文件/执行进程 | 沙箱无 os/io，用 `dsc.tool.call("shell" / "read_file", ...)` |
| `dsc.llm.chat` 返回空 | 已内部走流式；确认 `user` 必填、`system` 可省 |
| handler 返回 table 期望原样 | table 自动 JSON 序列化；要纯文本就返回字符串 |
| 依赖"自己工具内部调用的工具"经过自己的 before 钩子 | 嵌套调用时本脚本钩子会跳过（防死锁） |
| 忘了 `dsc.register_tool` | 只定义函数不注册 = 无工具产生 |
| store 键名冲突 | 键加脚本前缀，如 `myplugin_count` |
| 热加载后旧状态 | store 保留（进程内），但脚本内局部变量重载后重置 |

---

## 11. 完整示例（可直接复制）

### 示例 A：LLM 工具（翻译/摘要）

```lua
-- scripts/translator/main.lua
type TransArgs = { text: string, target: string? }

local function translate(args: TransArgs): string
    local target: string = args.target or "中文"
    local r: string = dsc.llm.chat({
        system = "你是专业翻译。只输出译文，不加解释。",
        user = "把下面内容翻译成" .. target .. "：\n" .. args.text,
        max_tokens = 500
    })
    return r
end

dsc.register_tool("translate", {
    description = "把文本翻译成指定语言（经宿主 LLM）",
    parameters = {
        type = "object",
        properties = {
            text = { type = "string", description = "待翻译文本" },
            target = { type = "string", description = "目标语言，默认中文" }
        },
        required = { "text" }
    }
}, translate)
```

### 示例 B：编排工具（多步骤 + 状态）

```lua
-- scripts/devops/main.lua
-- 收集目录信息 → 统计代码规模 → 汇报

local function code_stats(args: { dir: string }): string
    local ok, out = pcall(function()
        return dsc.tool.call("shell", { command = "dir /s /b " .. args.dir })
    end)
    if not ok then
        return "统计失败: " .. tostring(out)
    end
    dsc.store.set("devops_last_dir", args.dir)   -- 记住上次目录
    local count = 0
    for line in out:gmatch("[^\r\n]+") do
        count = count + 1
    end
    dsc.notify.emit("devops/stats", { dir = args.dir, files = count })
    return "目录 " .. args.dir .. " 共 " .. tostring(count) .. " 个文件。"
end

dsc.register_tool("code_stats", {
    description = "统计目录下文件数量并发布事件",
    parameters = {
        type = "object",
        properties = { dir = { type = "string", description = "目录路径" } },
        required = { "dir" }
    }
}, code_stats)
```

### 示例 C：后台任务 + 钩子

```lua
-- scripts/reporter/main.lua
-- 后台跑一次分析，完成后发事件；拦截危险 shell 命令

dsc.register_tool("start_analysis", {
    description = "后台启动代码分析，返回任务 id",
    parameters = { type = "object", properties = {} }
}, function()
    local job = dsc.job.spawn(function()
        local r: string = dsc.llm.chat({ user = "分析这个仓库的模块划分。", max_tokens = 300 })
        dsc.notify.emit("reporter/analysis_done", { result = r })
        return r
    end)
    return "分析任务 " .. job .. " 已启动，结果完成后发布 reporter/analysis_done 事件。"
end)

dsc.hook.before_tool(function(name: string, args: any): any
    if name == "shell" and args and args.command
        and (string.find(args.command, "rm -rf") or string.find(args.command, "format")) then
        return true, "危险命令已被 LUA 钩子拦截", nil
    end
    return false, "", nil
end)
```

---

## 12. 交付清单（完成一个 LUA 插件后自查）

- [ ] 目录 `scripts/<插件名>/main.lua` 已创建，命名符合规范
- [ ] 每个工具都有完整 `description` + 参数 Schema
- [ ] 参数/返回/局部变量写了类型注解
- [ ] 工具经热加载后 `lua_<工具名>` 可用，正常/边界/错误路径均验证过
- [ ] 用到的 dsc 服务（llm/tool/store/hook/job/notify）用法正确
- [ ] 沙箱限制内完成（文件/进程经 dsc.tool.call 转发）
- [ ] 向用户说明新增了哪些工具、如何调用、注意事项
