# DSC

本项目系使用 Golang 實現的編程代理系統，遵循 DeepSeek Harness 的一切皆插件的设计哲学，插件都可以热插拔方式加载或卸载。

## 核心功能

- **插件架構**：基於 `go-plugin` 与 gRPC 的宿主與插件通信機制，支持熱插拔加載或卸載。
- **熱重載（Hot Reload）**：支持 DSCPlugin、Agent、LLM 等類型插件的在線熱重載，無需重啟主程序即可更新插件版本。
- **多 LLM 支持**：支持 OpenAI、Anthropic、Ollama 等主流 LLM 提供商；其中 `llm-openai` 與 `llm-anthropic` 插件支持本地 LlamaCpp 推理引擎。
- **ReAct 循環**：實現 Agent 的 Reasoning and Acting 循環，支持多輪推理與工具執行。
- **工具調用與插件化**：支持通過 Tool 插件擴展工具集，內置文件操作與 shell 執行能力。沙箱策略三檔（对齐 DSH sandbox mode）：`read-only`（拒绝一切文件写）、`workspace-write`（仅允许在 workspace 根內写，默认）、`full-access`（不额外拦截）；TUI 内经 `/sandbox read-only | workspace | full-access` 运行时切换，workspace 根默认为启动 dsc 的目录（也可经 `workspace_root` 绝对路径覆盖）。各档下相对路径写始终以 workspace 为根（防止 `../` 路径穿越），绝对路径写 workspace 之外由沙箱策略统一管控。
- **RPC 可靠性保障**：跨插件 gRPC 調用支持超時控制與指數退避重試機制；採用語義化版本範圍（`>=1.0, <2.0`）進行插件 API 兼容性檢查，允許補丁與次版本升級。
- **TUI 工具調用顯示增強**：在 TUI 界面中，針對 `str_replace_editor`（或名稱包含 `editor` 的編輯器工具），會根據具體的 `command`（如 `view`, `create`, `str_replace`, `insert`）和 `path` 參數，顯示為 `Edit(View, /root/file/path)`、`Edit(Create, /root/file/path)`、`Edit(StrReplace, /root/file/path)`、`Edit(Insert, /root/file/path)` 等格式，提供更清晰的操作方向性展示。

## 與 DSH（DeepSeek Harness）的對比

DSC 與 DSH 同源於「一切皆插件」的設計哲學，兩者在概念層高度同構，但語言棧與運行形態不同：DSH 為 TypeScript / Node.js（cordis 插件框架），DSC 為 Go / go-plugin + gRPC。

### 核心功能對比

| 功能領域 | DSH（deepseek-harness） | DSC |
| --- | --- | --- |
| 宿主/插件架構 | cordis 插件框架，一切皆插件 | go-plugin + gRPC，一切皆插件，支持熱插拔/熱重載 |
| 沙箱隔離 | 內核級：bwrap（bind mount）/ Landlock（`sandbox-local` + 各 runner profile） | 宿主工具級攔截（Windows 兼容）：工具流水線 pre-execute 瀑布，三檔 `read-only / workspace-write / full-access` |
| 沙箱策略歸屬 | `ctx.sandboxPolicy` 單一歸屬（mode + workspace 根）；`renderPolicyContext` 以真實路徑呈現給模型 | 宿主 `Manager.sandboxPolicyVal` 單一歸屬；TUI `/sandbox` 即時切換；system prompt 注入 `sandbox:policy` 上下文（同樣以真實根路徑呈現） |
| 路徑圍欄 | `fs-sandbox` containment：詞法快速路徑（Windows 忽略大小寫）+ 文件身份（dev/ino）回退（識別 8.3 短名、大小寫別名） | `inWorkspace`：詞法前綴（Windows 忽略大小寫）+ 根路徑 `EvalSymlinks`；`/workspace` 虛擬前綴映射到統一根 |
| 會話 | 事件日誌（event log）+ `deriveMessages()` 派生模型歷史 | 事件溯源 `session` 包 + `DeriveMessages`（同構） |
| 上下文壓縮 | `compaction-basic`：thresholdRatio / retainRatio | 80% 閾值觸發、16% 尾部保留（≥1024 token）、字符估算兜底 |
| token 計量 | TokenMeter：精確 tokenizer，缺省字符估算回退 | 字符估算 + **提示緩存感知**（`input_tokens + cache_read_input_tokens`） |
| 歷史注入 | 無按條數限制機制（靠壓縮限界）；`maxMessages` 僅用於歷史查看分頁 | **`/settings history N\|off\|unlimited`** 會話級限制 + `DSC_HISTORY_INJECTION` |
| 技能 | `skill/` provider registry + catalog/loader tool（`ctx.skills`） | `skills/builtin` + `skills/installed`，`read_skill` 按需加載、`install_skill`/`uninstall_skill` 管理 |
| plan/goal/todo | plan-mode、goal、todo 領域 | 對齊：宿主托管 plan/goal/todo 工具，狀態經事件日誌折疊 |
| UI | Web UI（`apps/web`） | TUI（bubbletea）+ Web UI（`tool-harness-webui`） |

### 功能對齊程度

- **完全對齊（概念同構）**：事件溯源會話、沙箱三檔策略語義、上下文壓縮（pre-step 壓力檢查 + 尾部保留）、plan/goal/todo 領域、以真實路徑呈現 workspace 根給模型、技能注入。
- **部分對齊（同概念、異實現）**：沙箱從 DSH 的內核級（bwrap/Landlock）改為 DSC 的宿主工具級攔截（換取 Windows 兼容與可移植性，代價是「策略圍欄」而非「內核邊界」）；token 計量從精確 tokenizer 改為字符估算 + 提示緩存感知；技能注入從 DSH 的 provider registry 改為目錄掃描 + `ListContext` 索引。
- **DSC 擴展（DSH 沒有）**：`/settings history` 歷史注入條數限制（DSH 僅靠壓縮限界）；提示緩存感知的容量計算（本地 llama.cpp 緩存命中時 `input_tokens` 僅含新增部分，須加回 `cache_read`）；`/sandbox` TUI 即時切換；多會話 TUI 管理（`/session new\|list\|switch\|delete`）；cron 定時任務；`-input` 自動化多輪入口；`-debugger` 管理 API 觀察端點。

### 各自獨特實現

- **DSH 獨特**：內核級沙箱（真實 OS 邊界，不可信代碼經 `ctx.shell` 隔離）；跨能力族統一的可寫根集合（`writableRoots` 與 Seatbelt profile 共享，防止 fs 圍欄與 runner 漂移）；文件身份（dev/ino）圍欄回退；TokenMeter 精確計量；API 代理層（`api-proxy`：歷史分頁、子代理、投影）。
- **DSC 獨特**：純 Go + go-plugin/gRPC 全棧；TUI 交互（拖選複製、流式期間滾動、狀態行輪/步/容量）；Windows 兼容的工具級沙箱攔截；提示緩存感知的容量與壓縮判定；`/settings history` 歷史注入控制；事件溯源多會話 + `/session` 管理；cron 調度；`-debugger` 管理 API；`-input` 重定向多輪自動化。

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
- `tool-lisp-eval`
- `tool-skill`

### Policy 插件
- `policy-fs-observation`

## 構建與運行

使用提供的構建腳本編譯主程序與所有插件：

```bash
./build.sh
```

運行主程序並指定 LLM 提供商（默認為 openai）：

```bash
LLM_PROVIDER=openai ./dsc
```

## 許可證

本項目基於 [Apache-2.0 License](LICENSE) 許可。