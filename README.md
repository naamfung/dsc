# DSC

本项目系使用 Golang 實現的編程代理系統，遵循 DeepSeek Harness 的一切皆插件的设计哲学，插件都可以热插拔方式加载或卸载。

## 核心功能

- **插件架構**：基於 `go-plugin` 与 gRPC 的宿主與插件通信機制，支持熱插拔加載或卸載。
- **熱重載（Hot Reload）**：支持 dsc / agent / llm / tool / policy 五類插件的在線熱重載，無需重啟主程序即可更新插件版本；幾乎全程不持全局鎖，失敗不影響舊實例。
- **多 LLM 支持**：支持 OpenAI、Anthropic、Ollama 等主流 LLM 提供商；其中 `llm-openai` 與 `llm-anthropic` 插件支持本地 LlamaCpp 推理引擎。
- **ReAct 循環**：實現 Agent 的 Reasoning and Acting 循環，支持多輪推理與工具執行。
- **工具調用與插件化**：支持通過 Tool 插件擴展工具集，內置文件操作與 shell 執行能力。沙箱策略三檔（对齐 DSH sandbox mode）：`read-only`（拒绝一切文件写）、`workspace-write`（仅允许在 workspace 根內写，默认）、`full-access`（不额外拦截）；TUI 内经 `/sandbox read-only | workspace | full-access` 运行时切换，workspace 根默认为启动 dsc 的目录（也可经 `workspace_root` 绝对路径覆盖）。各档下相对路径写始终以 workspace 为根（防止 `../` 路径穿越），绝对路径写 workspace 之外由沙箱策略统一管控；`read-only` 同时会禁用「命令无法从参数判定是否只读」的解释器/执行器（如 shell），防止 `echo x > /anywhere` 绕开只读档。
- **工具調用超時（活躍續命）**：shell 与命令型工具采用「十分鐘起步、活躍續命」超时（对齐 rex shell）——启动 10 分钟预算（`DSC_SHELL_TIMEOUT` 可覆盖），只要 stdout/stderr 持续有新输出就不间断续命，仅对「长时间完全无新输出」才判定超时，避免一刀切固定时长方误杀仍在产出嘅长编译/测试。
- **凭据隔离**：插件子进程 env 白名单化——仅 LLM 插件放行凭据类键（`*_API_KEY`/`*_TOKEN`/`*_SECRET` 等），其余 tool/policy/agent 插件一律滤除（`DSC_*` 宿主配置保留），防止 API key 经 shell 等工具进程被模型读进会话历史。
- **RPC 可靠性保障**：跨插件 gRPC 調用支持超時控制與指數退避重試機制；採用語義化版本範圍（`>=1.0, <2.0`）進行插件 API 兼容性檢查，允許補丁與次版本升級。
- **TUI 工具調用顯示**：所有工具名在 TUI 統一以 PascalCase 呈現（`read_skill` → `ReadSkill`、`update_goal` → `UpdateGoal`、`shell` → `Shell`）；針對 `str_replace_editor`（或名稱包含 `editor` 的編輯器工具），會根據具體的 `command`（如 `view`, `create`, `str_replace`, `insert`）和 `path` 參數，顯示為 `StrReplaceEditor(View, /root/file/path)`、`StrReplaceEditor(Create, /root/file/path)` 等格式。工具结果的展示采用**结构化视图声明**（对齐 DSH 显示契约）：实现工具的插件可选地在结果里声明**结构化视图 spec**（`ExecuteToolResponse.view_json`，见 SDK 的 `dsc.CardView` / `dsc.TableView` / `dsc.PlainView`），TUI 以**单一渲染器统一绘制**三种版式——**card**（标题 + 语义色徽标 + 对齐键值字段）、**table**（对齐列头 + 对齐行，超长单元格截断）、**plain**（标题/徽标 + 正文块），保证各工具结果风格一致。已内置插件的各工具均已按此实现专属视图（goal/todo/ask 卡片与表格、memory_search/ssh_list/web_search 表格、fetch/read_skill/lisp_eval/ssh_exec/shell 纯文本、ssh/skill/browser 卡片等），宿主侧 `run_code` 亦有专属视图（RunCode plain 块，徽标为 stop_reason，正文为返回值/错误）；宿主聚合 Tool 服务会把插件 `ViewJson` 与宿主工具视图一并透传到 `ExecuteToolResponse.view_json`。未声明视图的工具回退到通用 JSON 键值卡片（同层值列对齐），再退回到原文（`str_replace_editor` 的 diff 着色由通用兜底保留）。browser-use 插件支持 `DSC_BROWSER_CDP_URL` 指向既有 CDP 端点（跳过本地 chromium 启动，供 mock chromium 集成测试等场景）。
- **多 Agent 工作流（workflow）**：宿主内置 `workflow` 模型工具（对齐 DSH tool-workflow）——模型编写的 Lua 编排脚本，由 go-lua 的**协程排程器**执行，可扇出 subagent（`agent`/`parallel`/`pipeline`/`phase`/`log` 钩子）；子代理默认无迭代上限，何时完成由模型自行决定；`return` 的 JSON 即结果；支持 `background: true` 后台运行。TUI `/jobs list | output <id> | kill <id>` 用户命令管理后台任务，模型亦可用 `job_output`/`job_list`/`job_kill` 工具。
- **程序化工具呈现（PTC）**：宿主内置 `run_code` 工具（对齐 DSH 的 PTC 概念）——模型写一段**严格 Lua** 程序一把过组合多步工具调用，而不再逐个 call：程序里每个可用工具以同名 Lua 函数呈现（`mytool{...}`），顶层 `return` 即结果；语言是带类型注解、可空 `T?`、联集、流式收窄的受检方言（基于 go-lua）。`-mode ptc`（或 `DSC_PTC=1`）开启**呈现模式**：把直接工具调用**折叠**为唯一 `run_code`，其余工具仅经其程序内 SDK 可调（对齐 DSH presentation；native/其余模式下 `run_code` 对模型隐藏、也不可执行）；system prompt 引入 PTC 引导，`run_code` 描述携带「程序内可调工具」清单与严格 Lua 方言规范，助模型一把过组合多步。
- **項目級歷史隔離**：默认会话按当前工作区项目路径命名（`C:\...\DeepClean` → `C--...-DeepClean.jsonl`），同项目跨时期共享历史、不同项目隔离，不再使用硬编码 `default.jsonl`；TUI 的当前会话标识与切换也统一对齐该项目 key（宿主 `DefaultSessionID()` 与 agent 的 `projectKey` 同源），`/session default` 解析到项目 key、`/export` 导出真实存档，且**标题栏不再显示会话 id**（经 `/sessions` 列表与 `/export` 管理，避免「显示名 ≠ 存档名」的脱节）；`/settings history <N|off|unlimited>` 实时生效并持久化到 config.yaml（`history_injection`：-1 禁止 / 0 未定义 / N>0 启用 N 条）。
- **沙箱范围可见**：TUI 左下角状态栏随 `/sandbox` 即时显示当前工作范围——`full-access` 显示「文件系统」，其余显示工作区目录基础名（限长）。
- **事件體系（對齊 DSH harness 事件）**：宿主 EventBus 採與 DSH cordis events 一致的五種分發模式（`emit` 廣播通知 / `waterfall` 洋葱攔截 / `serial` 順序 / `bail` 短路 / `parallel` 並發），並經互通機制把宿主事件廣播給插件（`Hook.OnEvent`），**不限定插件類型**——任意註冊了 Hook 的插件（tool/dsc/llm/agent/policy）都能訂閱，對齊 DSH cordis 的「事件廣播類型無關」；令插件可獨立訂閱系統事件而不改宿主。已對齊的關鍵事件：工具流水線 `tools/pre-execute` / `tools/execute` / `tools/post-execute`（waterfall 攔截，veto 即阻止；execute 供插件包圍執行與超時策略）與 `tools/result`（emit 結果廣播）；agent 回合生命週期 `agent/status`（running/idle）與 `agent/error`（emit，成功/失敗區分）。因 DSC 的 agent 為獨立 gRPC 插件進程（不同於 DSH 宿主內循環），僅對齊機制與有真實消費者的事件，不機械照搬無消費者或需跨進程空轉的事件。這些領域事件與運行時日誌可經管理 API 的 SSE 端點實時觀測：`/plugins/domain-events`（推送全部 EventBus 領域事件，含字段保真載荷）與 `/plugins/logs`（推送宿主日誌與插件子進程經轉發上來的日志；宿主與插件 logger 統一接入扇出 sink，即使默認靜默模式也按需可察）。`-input` 非 headless 且已加載通知插件（如 notify）時，回合結束後會短暫寬限（約 0.8s）再關閉插件，確保異步的回合完成音效（約 0.29s）能完整播完，避免被插件進程回收截斷。

- **插件安裝/管理（模型可自助）**：宿主內置 `install_go_plugin` / `uninstall_go_plugin` / `list_go_plugins` 三個模型工具（對齊 SKILL 安裝，經聚合 Tool 服務暴露給模型）。嚴格命約定（插件目錄 `plugins/<type>-<name>/`、執行檔 `<type>-<name><ext>`、`type`∈tool/llm/agent/policy/dsc、`name` 僅 `[A-Za-z0-9_-]`），寫 config 前先備份（`config.yaml.<ts>.bak`），並「干跑=live 加載」驗證類型/元數據一致才落盤、失敗回滾（刪已拷目錄、config 未變），防止模型寫壞配置導致起不來。新裝插件即時熱加載、無需重啟；模型據「能否加載 ACTIVE」判斷安裝是否正確，可再用 `uninstall_go_plugin` 清理。

## 與 DSH（DeepSeek Harness）的對比

DSC 與 DSH 同源於「一切皆插件」的設計哲學，兩者在概念層高度同構，但語言棧與運行形態不同：DSH 為 TypeScript / Node.js（cordis 插件框架），DSC 為 Go / go-plugin + gRPC。

### 核心功能對比

| 功能領域 | DSH（deepseek-harness） | DSC |
| --- | --- | --- |
| 宿主/插件架構 | cordis 插件框架，一切皆插件 | go-plugin + gRPC，一切皆插件，支持熱插拔/熱重載 |
| 沙箱隔離 | 內核級：bwrap（bind mount）/ Landlock（`sandbox-local` + 各 runner profile） | 宿主工具級攔截（Windows 兼容）：工具流水線 pre-execute 瀑布，三檔 `read-only / workspace-write / full-access` |
| 沙箱策略歸屬 | `ctx.sandboxPolicy` 單一歸屬（mode + workspace 根）；`renderPolicyContext` 以真實路徑呈現給模型 | 宿主 `Manager.sandboxPolicyVal` 單一歸屬；TUI `/sandbox` 即時切換；system prompt 注入 `sandbox:policy` 上下文（同樣以真實根路徑呈現） |
| 路徑圍欄 | `fs-sandbox` containment：詞法快速路徑（Windows 忽略大小寫）+ 文件身份（dev/ino）回退（識別 8.3 短名、大小寫別名） | `inWorkspace`：`CanonicalPath` 解析真實路徑（Windows 用 `GetFinalPathNameByHandle` 穿透 junction/symlink，Unix 用 EvalSymlinks）再做包含判定，防 workspace 內指向外部的連結寫穿；`/workspace` 虛擬前綴映射到統一根 |
| 會話 | 事件日誌（event log）+ `deriveMessages()` 派生模型歷史 | 事件溯源 `session` 包 + `DeriveMessages`（同構） |
| 上下文壓縮 | `compaction-basic`：thresholdRatio / retainRatio | 80% 閾值觸發、16% 尾部保留（≥1024 token）、字节级启发式估算兜底 |
| token 計量 | TokenMeter：本地精確 tokenizer，缺省字符估算回退 | 以服務端上報 usage（精確）為準；服務端不可用（重啟）或低估（提示緩存命中）時回退字节级启发式估算 + 提示緩存感知（`input_tokens + cache_read_input_tokens`） |
| 歷史注入 | 無按條數限制機制（靠壓縮限界）；`maxMessages` 僅用於歷史查看分頁 | **`/settings history N\|off\|unlimited`** 項目級會話隔離（按工作區命名）+ 持久化到配置（`history_injection` 三態） |
| 技能 | `skill/` provider registry + catalog/loader tool（`ctx.skills`） | `skills/builtin` + `skills/installed`，`read_skill` 按需加載、`install_skill`/`uninstall_skill` 管理 |
| plan/goal/todo | plan-mode、goal、todo 領域 | 對齊：宿主托管 plan/goal/todo 工具，狀態經事件日誌折疊 |
| UI | Web UI（`apps/web`） | TUI（bubbletea）+ Web UI（`tool-harness-webui`） |

**PTC（程序化工具呈现）差异**：两者概念同构，且呈现方式现也已对齐——模型写一段程序组合多步工具调用、一把过执行；`ptc` 下把直接工具调用**折叠为唯一 `run_code`**（其余工具仅经其程序内 SDK 可调），native 模式则对模型**隐藏 `run_code`、也不可执行**（run_code-only 的高隐藏）。仅实现语言不同：DSH 的原生 PTC 用 **TypeScript**（runtime 本身 TS/Node）；DSC 用 **严格 Lua**（go-lua，带类型注解、可空 `T?`、联集、流式收窄的受检方言）。

### 功能對齊程度

- **完全對齊（概念同構）**：事件溯源會話、沙箱三檔策略語義、上下文壓縮（pre-step 壓力檢查 + 尾部保留）、plan/goal/todo 領域、以真實路徑呈現 workspace 根給模型、技能注入。
- **部分對齊（同概念、異實現）**：沙箱從 DSH 的內核級（bwrap/Landlock）改為 DSC 的宿主工具級攔截（換取 Windows 兼容與可移植性，代價是「策略圍欄」而非「內核邊界」）；雖為工具級，但對已知寫路徑、Windows junction/symlink 穿越、以及不可定位寫路徑的解釋器逃逸，均已於工具流水線 pre-execute 階段 fail-closed 封堵（見下方「各自獨特實現」）；token 計量從 DSH 的 TokenMeter（本地精確 tokenizer）改為 DSC 的「服務端 usage + 字节级启发式估算回退」；技能注入從 DSH 的 provider registry 改為目錄掃描 + `ListContext` 索引。
- **DSC 擴展（DSH 沒有）**：`/settings history` 歷史注入條數限制（DSH 僅靠壓縮限界）+ 項目級會話隔離 + 配置持久化；提示緩存感知的容量計算（本地 llama.cpp 緩存命中時 `input_tokens` 僅含新增部分，須加回 `cache_read`）；`/sandbox` TUI 即時切換；多會話 TUI 管理（`/session new\|list\|switch\|delete`）；cron 定時任務；多 Agent workflow 後台運行（`background: true`）+ TUI `/jobs` 管理命令；`-input` 自動化多輪入口；`-headless` 精简单发模式；`-debugger` 管理 API 觀察端點；管理 API 的 `/plugins/domain-events` 與 `/plugins/logs` SSE 流實時觀測領域事件與宿主/插件日誌。

### 各自獨特實現

- **DSH 獨特**：內核級沙箱（真實 OS 邊界，不可信代碼經 `ctx.shell` 隔離）；跨能力族統一的可寫根集合（`writableRoots` 與 Seatbelt profile 共享，防止 fs 圍欄與 runner 漂移）；文件身份（dev/ino）圍欄回退；TokenMeter 精確計量；API 代理層（`api-proxy`：歷史分頁、子代理、投影）。
- **DSC 獨特**：純 Go + go-plugin/gRPC 全棧；TUI 交互（拖選複製、流式期間滾動、狀態行輪/步/容量）；Windows 兼容的工具級沙箱攔截（pre-execute 三檔策略 + junction 穿越與解釋器逃逸的 fail-closed 封堵）；提示緩存感知的容量與壓縮判定；`/settings history` 歷史注入控制；事件溯源多會話 + `/session` 管理；cron 調度；`-debugger` 管理 API；`-input` 重定向多輪自動化；`-headless` 精简单发（仿 DSH harness headless）。

## 支持的插件

### LLM 插件
- `llm-openai`
- `llm-anthropic`
- `llm-ollama`

### Agent 插件
- `agent-react-loop`

### Tool 插件
- `tool-filesystem`
- `tool-str-replace-editor`
- `tool-browser-use`
- `tool-lisp-eval`（Lisp/Scheme 精确有理数求值：`+ - * /` 变参精确运算、`3/4` 分数字面量、任意精度整数；浮点走 `f+ f- f* f/` 逃生舱）
- `tool-skill`
- `tool-lua-host`（LUA 脚本宿主：脚本注册工具，宿主互通复用 LLM/Tool/Notify）
- `tool-memory-service`（记忆库工具：原生 RPC 工具 + AfterTool 自动记忆钩子，落点宿主可执行目录 `memory/`，跨会话共享）
- `tool-harness-webui`（独立 HTTP 服务，代理宿主 admin API 的前端）

### Policy 插件
- `policy-fs-observation`

### DSC 通用插件
- `dsc-notify`（通知音效插件：**通用 dsc 类型**，纯后台程序性驱动、不暴露模型工具——经 Hook.OnEvent 订阅宿主通用 agent 回合事件，成功（`agent/status` idle）播 success、失败（`agent/error`）播 error，无需模型调用；内置音效 success/error/warning/info 与自定义 `.mp3/.wav`）

## 目錄結構

- `core/` — 宿主核心（插件管理、工具流水線、sandbox、subagent、workflow 工具等）
- `plugin/` — 定制版 go-plugin 庫（module path 仍 `github.com/hashicorp/go-plugin`，含宿主掛載聚合服務必需的 `GRPCClient.Broker()` 擴展）
- `plugins/` — 各插件實現（`llm-*` / `tool-*` / `agent-*` / `policy-*` / `dsc-*`）
- `sdk/` — `dsc-sdk`：聲明式插件構建器（獨立 module，插件作者只需導入 SDK）
- `workflow/` — 多 Agent Lua 编排引擎（go-lua 协程排程器执行模型编写的脚本）
- `coderuntime/` — `run_code` 的实现：go-lua 隔离执行程序 + 按工具目录生成 Lua SDK
- `lualib/` — go-lua 值与 Go/JSON 互转的共享工具包（coderuntime 与 workflow 共用，避免重复实现）
- `jobs/` — 後台任務註冊表（workflow 後台運行承載）
- `session/` — 事件溯源會話（按項目路徑命名存儲，跨項目隔離）
- `proto/` — gRPC 定義與生成代碼
- `tui/` — 終端界面（Bubble Tea）
- `libs/` — vendored 本地依赖 fork（`go-lua`、`jig-lisp` 精确有理数解释器等），经各插件 `go.mod` 的 `replace` 指入

## TUI 斜杆命令

| 命令 | 用途 |
| --- | --- |
| `/sandbox read-only \| workspace \| full-access` | 切換沙箱策略（狀態欄即時顯示工作範圍） |
| `/settings history <N\|off\|unlimited>` | 歷史注入條數（實時生效並持久化到配置） |
| `/jobs [list\|output <id>\|kill <id> [reason]]` | 管理後台任務（含 workflow） |
| `/session <id>\|new\|delete <id>` · `/sessions` | 多會話管理 |
| `/cron add\|remove\|on\|off` · `/crons` | 定時任務 |
| `/plan [off]` | plan 模式開關 |
| `/mode minimal\|standard\|creation\|ptc` | 切換模式（`ptc` 开启 PTC 程序化工具组合呈现：直接把工具调用**折叠**为唯一 `run_code`，其余工具仅经其程序内 SDK 可调；其余模式为 native，`run_code` 对模型隐藏） |
| `/skills` · `/help` · `/clear` · `/export` · `/mouse` | 技能 / 幫助 / 清屏 / 導出 / 鼠標 |

## 啟動參數（CLI 旗標）

| 旗標 | 用途 |
| --- | --- |
| `setup`（子命令） | 交互式配置向导：基于 config.yaml 的插件状态动态发现 LLM 提供商（并补充扫描 plugins 目录中未声明的 `llm-*` 插件），以行式菜单编辑基址/模型/API key、设置默认提供商，写回 config.yaml（保留注释），避免手动改配置文件的格式风险。仅配置 LLM 连接与 `default_llm`，为快速启动到可用状态；其他设置仍走 config.yaml |
| `-mode minimal\|standard\|creation\|ptc` | 切換模式（默認 `standard`；`ptc` 开启 PTC 程序化工具组合呈现，直接把工具调用折叠为唯一 `run_code`；其余模式 `run_code` 对模型隐藏） |
| `-input <text>` | 非 TUI 自動化入口：執行一單輪後退出；stdin 為管道/文件重定向時進入多輪 stdin 驅動，直到 EOF |
| `-headless` | 精简单发模式（对齐 DSH harness headless）：仅执行 `-input` 指定的**单个任务**一次后退出，不启动后续 stdin 多轮；任务须非空白（否则 stderr 报错并以码 1 退出）；**不开** ADMIN API 端口、热重载 watcher 与 cron，专为 CI 脚本 |
| `-admin <addr>` | 管理 API 监听地址（缺省取环境变量 `DSC_ADMIN_ADDR`，再默认回环 `127.0.0.1:9999`；需远程管理时用 `-admin :9999` 并配置 `DSC_ADMIN_TOKEN`）。未配置 `DSC_ADMIN_TOKEN` 不开认证 |
| `-debugger` | 开放 `/debugger` 观察路由（含完整会话历史，敏感，默认不开放） |
| `-log [<file>]` | 日志：带文件名写文件；仅 `-log` 时输出到屏幕 |

## 構建與運行

使用提供的構建腳本編譯主程序與所有插件：

```bash
./build.sh
```

清理构建产物（开发用）：删除主程序二进制与 plugins/ 目录下所有插件构建产物，
但不触碰任何源码 / 配置 / 文档：

```bash
./clean.sh
```

運行主程序並指定 LLM 提供商（默認為 openai）：

```bash
LLM_PROVIDER=openai ./dsc
```

## Golang 插件熱更新實操

DSC 的 Go 插件（基於 go-plugin / gRPC）支持**版本化二進制的在線熱更新**：無需重啟宿主，即可把運行中的 dsc / agent / llm / tool / policy 插件換成新版本進程。

### 原理

- **啟動選版**：`LoadFromConfig` 用 `ResolveLatestBinary` 在每個插件的二進制目錄內挑選「版本號最高」的版本化文件作為初始載入（見 [core/hot_reload_version.go](core/hot_reload_version.go)）。
- **運行監測**：配置 `hot_reload: true` 時，宿主經 `StartHotReloadWatcher` 啟動 fsnotify + 週期掃描（見 [core/hot_reload_watch.go](core/hot_reload_watch.go)）。一旦某插件目錄內出現比當前運行版本更高的 `<插件名>-v<版本><擴展名>` 文件（fsnotify 即時 + ≤5s 週期兜底，≥500ms 節流防抖），即自動調用 `HotReload` 換進程，不中斷宿主與其他插件。
- **支援類型**：dsc / agent / llm / tool / policy 五類全部可熱重載。
- **原子「暂存 + 提交」**：dsc / agent / llm / tool / policy 五類插件均採用兩階段熱重載。先在不持鎖階段拉起新進程並完成全部慢速 RPC（handshake、broker 掛載、互通注入、工具列清單/策略對齊，agent 另含依賴注入），確證體康後才在極短臨界持鎖區一次性交換地圖引用並殺舊進程；預備/驗證任一環節失敗即中止並 Kill 新進程，**舊實例及其註冊原封不動**。

### 版本化文件命名約定

格式：`<插件基名>-v<主版本>.<次版本>[.<修訂>[.<構建>]]<擴展名>`

- **基名取自行為所在目錄的基名**（即插件名），版本化文件須與現行運行文件**同目錄**。
- 舉例（tool-filesystem，Windows 下為 `.exe`）：

  ```
  plugins/tool-filesystem/tool-filesystem.exe          # 基線（視為 0.0.0）
  plugins/tool-filesystem/tool-filesystem-v1.2.0.exe
  plugins/tool-filesystem/tool-filesystem-v4.0.1.exe
  ```

- 版本比較採用語義化版本（semver，經 `hashicorp/go-version`），多個版本化文件共存時宿主取版本號**最高**者。
- **為何用「版本號文件」而非覆蓋原文件**：Windows 下運行中的 `.exe` 被進程鎖定、無法原地覆蓋。寫成新版本文件後宿主直接以新文件啟動新進程，舊文件留在磁碟待資源釋放後由人手清理。

### 開啟方法

在 `config/config.yaml` 增加並重啟宿主一次（首次開啟需重啟才能啟動 watch；此後插件更新皆不需重啟）：

```yaml
hot_reload: true
```

### 一次插件更新的實操流程

1. 修改插件源碼（例如 `plugins/tool-filesystem`）。
2. 在插件目錄內把新二進制編譯成**自增版本號**的文件名：

   ```bash
   go build -o plugins/tool-filesystem/tool-filesystem-v2.3.1.exe ./plugins/tool-filesystem
   ```

3. 宿主較短時間內檢測到更高版本，日誌輸出 `hot-reload detected higher version binary ...`；**先拉起並驗證新進程，成功後才同步卸載（殺）舊進程**並以 `-v2.3.1.exe` 持續提供服務；新進程準備/驗證任一環節失敗即中止並 Kill 新進程，**舊實例及其註冊原封不動**。
4. 確認日誌 `hot-reload applied` 且新行為生效。

### 注意事項

- 版本號必須嚴格符合 `v<主>.<次>[.<修訂>[.<構建>]]`；`v1`、`v1.2.3.4.5` 無法匹配，`v<num>.<num>` 為最小合法形式。
- 版本化文件基名須**恰好等於二進制所在目錄的基名**，否則不會被識別為該插件的更新。
- 宿主不自動刪除舊版本化文件；舊進程退出後可自行清理殘留文件。

## 插件生命周期與依賴拓撲狀態機

宿主對每個插件維護一個運行狀態機（見 [core/lifecycle.go](core/lifecycle.go)），並把每次狀態遷移作為事件對外廣播（見 [core/events.go](core/events.go)），供 Admin/TUI 實時訂閱。

### 狀態值

| 狀態 | 含義 | 對應 DSH |
| --- | --- | --- |
| `PENDING` | 配置已聲明但依賴未滿足（如 DependsOn 的 LLM/Tool 尚未就緒），尚不拉起子進程 | PENDING |
| `SPAWNED` | 子進程已創建，尚未握手 | （DSH 無直譯） |
| `CONNECTING` | go-plugin/gRPC 握手、建鏈中 | LOADING 前半段 |
| `READY` | 業務對象已 Dispense 並註冊到 Manager，依賴/健康檢查尚未就緒 | （DSH 無直譯） |
| `ACTIVE` | 依賴與健康檢查就緒，可對外服務 | ACTIVE |
| `UNLOADING` | 卸載中，嘗試優雅關閉（stop hooks 先執行） | UNLOADING |
| `DISPOSED` | 已停止/已卸載（終態，不可再啟，除非重新加載） | DISPOSED |
| `FAILED` | 加載或運行失敗（終態，可被熱重載重新走一遍流程） | FAILED |

### 合法遷移

非法遷移會被記錄告警，便於及早暴露流程漏步：

```
PENDING    → SPAWNED / CONNECTING / ACTIVE / FAILED / DISPOSED
SPAWNED    → CONNECTING / FAILED / DISPOSED
CONNECTING → READY / FAILED / DISPOSED
READY      → ACTIVE / PENDING / UNLOADING / FAILED / DISPOSED
ACTIVE     → UNLOADING / FAILED / DISPOSED
UNLOADING  → DISPOSED / FAILED
DISPOSED / FAILED（終態，不再遷移）
```

### 啟動到結束的完整流程

1. **聲明與環檢**：`LoadFromConfig` 先以 `CheckCircularDependencies` 攔截環形依賴，再統計「已啟用插件集」供依賴判定（見 [core/manager.go](core/manager.go)）。
2. **Agent 先行**：agent 作為 broker 提供者**最先**拉起子進程以取得 broker（狀態 `SPAWNED → CONNECTING → READY`），但暫不激活——它依賴的 LLM/聚合 Tool 服務要等 provider 就緒後才掛載，避免 broker ConnInfo 超時窗口（宿主已把庫默認 5 秒放大為 5 分鐘，見 `PLUGIN_BROKER_CONN_TIMEOUT`）。
3. **Provider 依賴拓撲排序**：其餘 llm/tool/policy 依 `DependsOn` 做穩定拓撲排序（Kahn，`topoSortPlugins`）；依賴滿足的按序加載（LLM 原生加載後掛載為 broker 上的 gRPC 服務；Tool/Policy 走 `loadPluginWithBroker`），依賴未滿足的進入 `PENDING` 並記錄待辦。
4. **握手與校驗**：provider 加載時 `SPAWNED → CONNECTING`（建鏈）→ `READY`（元數據校驗：API 版本 `>=1.0, <2.0` + 類型一致；Tool 再經「暫存 + 提交」兩階段完成 broker 掛載、互通注入與工具列清單）。
5. **聚合服務與 Agent 激活**：provider 全部就緒後統一掛載聚合 LLM、聚合 Tool、插件通知與用戶評審服務，再依 agent 的 `DependsOn` 一次性 `RegisterServices` 注入並置 `ACTIVE`；若 agent 聲明的 LLM 缺失則退回 `PENDING` 等待。
6. **運行期**：插件以 `ACTIVE` 對外服務；故障進入 `FAILED`（可被熱重載重新走流程）；熱重載採用「暫存 + 提交」兩階段，先拉起並驗證新進程，成功後才交換並卸載舊進程，預備/驗證任一環節失敗即中止並 Kill 新進程，**舊實例及其註冊原封不動**（見「Golang 插件熱更新實操」）。
7. **卸載/關機**：`Shutdown` 先停熱重載 watch 與 cron，再逐個插件 `ACTIVE → UNLOADING`（先執行對稱清理 stop hooks，如 agent 的 `Shutdown`）→ Kill 子進程 → `DISPOSED`（終態）。
8. **動態注入與 PENDING 修復**：運行期經 ADMIN `/plugins/load` 注入的條目若依賴未滿足同樣進入 `PENDING`；後續注入補足缺口後，`repairPendingLocked` 會提升等待中的 provider、並把因缺 LLM 而 `PENDING` 的 agent 重新注入 `RegisterServices` 並激活（見 [core/inject.go](core/inject.go)）。

## 許可證

本項目基於 [Apache-2.0 License](LICENSE) 許可。