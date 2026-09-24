# DSC

本項目系使用 Golang 實現的編程代理系統，遵循 DeepSeek Harness 的一切皆插件的設計哲學，插件都可以熱插拔方式加載或卸載。

## 克隆指南

普通用户請直接下載正式版二進制程序，開發請克隆 **master** 分支：

```bash
git clone -b master https://github.com/naamfung/dsc.git
```

## 核心功能

- **插件架構**：基於 `go-plugin` 與 gRPC 的宿主與插件通信機制，支持熱插拔加載或卸載。

- **熱重載（Hot Reload）**：支持 dsc / agent / llm / tool / policy 五類插件的在線熱重載，無需重啟主程序即可更新插件版本；幾乎全程不持全局鎖，失敗不影響舊實例。

- **多 LLM 支持**：支持 OpenAI、Anthropic、Ollama 等主流 LLM 提供商；其中 `llm-openai` 與 `llm-anthropic` 插件支持本地 LlamaCpp 推理引擎。

- **ReAct 循環**：實現 Agent 的 Reasoning and Acting 循環，支持多輪推理與工具執行。

- **工具調用與插件化**：支持通過 Tool 插件擴展工具集，內置文件操作與 shell 執行能力。沙箱策略三檔（對齊 DSH sandbox mode）：`read-only`（拒絕一切文件寫）、`workspace-write`（僅允許在 workspace 根內寫，默認）、`full-access`（整個文件系統皆可寫）；TUI 內經 `/sandbox read-only | workspace | full-access` 運行時切換，workspace 根默認為啓動 dsc 的目錄（也可經 `workspace_root` 絕對路徑覆蓋），該根一律**歸一為正斜杆**後再注入各插件進程、拼進工具結果與沙箱提示——避免 Windows 反斜杆（如 `os.Getwd()` 的原生形態）流入模型上下文、誘導模型照抄反斜杆寫路徑。各檔下相對路徑寫始終以 workspace 為根（防止 `../` 路徑穿越），絕對路徑寫 workspace 之外由沙箱策略統一管控；`read-only` 同時會禁用「命令無法從參數判定是否只讀」的解釋器/執行器（如 shell），防止 `echo x > /anywhere` 繞開只讀檔。

- **工具調用超時（活躍續命，策略插件化）**：`dsc-system` 駐留超時策略（自 `policy-timeout` 遷入）在工具流水線 `tool/execute` 槽裁決「哪個工具、多大空閒預算、超時模型可見文案」（對齊 DSH timeout-policy 佔位）：`shell` 預算由 `DSC_SHELL_TIMEOUT`（默認 10 分鐘）、`subagent` 由 `DSC_SUBAGENT_IDLE_TIMEOUT`（默認 10 分鐘）可調，設 `0s` 禁用；宿主機械安裝執行域（看門狗 + 活動信號通道），執行方每次輸出/幀到達即續命，僅對「長時間完全無活動」才判定超時，避免一刀切固定時長誤殺仍在產出的長編譯/測試與慢速本地模型；cron 任務與 workflow 子代理亦經同一 `subagent` 工具裁決路徑，無旁路。

- **超長結果外置（spill，策略插件化）**：`dsc-system` 駐留外置策略（自 `policy-spill` 遷入）在工具流水線 `tool/post-execute` 槽裁決「超長純文本工具結果何時外置」（對齊 DSH spill-policy 的結果變換器佔位）：超過 `DSC_SPILL_THRESHOLD`（默認 4000 字符，設 `0` 禁用）的結果全文保存到外置存儲（`DSC_SPILL_DIR` 顯式覆蓋，缺省 exe 目錄 `temp/spill/<session>`，宿主 24 小時清理覆蓋），模型側只見「頭尾預覽 + 定位符 + 取回指引」；定位符即文件路徑，取回走標準 `str_replace_editor` view 命令（支持 `view_range` 分段）或 `shell` grep 搜索（view 命令豁免外置，防「取回 → 又被外置」死循環）；替換體永不超閾值（告示成本在閾值內預留）；盡力而為：存儲失敗/閾內容不下替換體一律保留內聯，絕不把成功調用變成失敗。

- **重複調用提醒（advisory，策略插件化）**：核心插件混合體 `dsc-system` 的駐留策略在工具流水線 `tool/post-execute` 槽觀察每次調用（對齊 DSH repeat-tool-reminder 的 advisory 形態）：以完全相同的規範化參數連續調用同一工具的次數達到閾值（`DSC_REPEAT_THRESHOLDS`，默認 `3,5,8`，首個閾值為簡短提醒、後續為列出工具/連擊次數/參數預覽的詳細提醒，`DSC_REPEAT_PREVIEW_CHARS` 默認 500 限預覽長度）時，經裁決的 advisory 通道（notice）注入提醒——不否決、不改寫工具結果，提醒在結果之後以合成用户消息投餵模型；被拒絕/失敗的調用同樣計數（反覆硬敲被拒調用正是最值得打斷的循環），`DSC_REPEAT_EXCLUDE`（默認 `todo_write`）對鏈透明，用户插話（回合新輸入或運行中注入）自動重置循環鏈。多個策略可共駐 `dsc-system` 單一程序（`plugins/dsc-system`，服務正交聲明 `PluginInfo.services`），後續核心插件將逐步遷移至此。

- **憑據隔離**：插件子進程 env 白名單化——僅 LLM 插件放行憑據類鍵（`*_API_KEY`/`*_TOKEN`/`*_SECRET` 等），其餘 tool/policy/agent 插件一律濾除（`DSC_*` 宿主配置保留），防止 API key 經 shell 等工具進程被模型讀進會話歷史。

- **上下文壓縮（compaction-basic，插件化）**：宿主壓縮引擎整體遷入 `dsc-system` 駐留（對齊 DSH compaction-basic，協議與 agent 零改動）：`agent/pre-step` 槽壓力驅動——壓力判定優先取「最近一次服務端上報用量 + 自上次請求以來改寫列表的啓發式增量」（對齊 DSH tokenMeter 的 projectedTokens；無上報記錄——首次請求/重啓恢復——退回純啓發式估算），估算統一複用 `core/token_meter.go` 公用啓發式（`EstimateProtoMessageTokens`/`EstimateProtoMessagesTokens`，CJK 感知 max(bytes/4, runes) + 角色開銷 + 圖像 + 工具參數；宿主 pre-step、agent 內聯壓縮同口徑），超閾值（默認窗口 80%，`DSC_COMPACTION_BASIC_THRESHOLD` 可調，對齊 compaction-basic；agent 經 `[DSC_COMPACTION_BACKEND_ACTIVE]` 後端標記檢測自動跳過內聯壓縮，接管靠能力聲明驅動、與閾值無關）時保留尾部（16%、≥1024 token）並把未壓縮前段經聚合 LLM 生成摘要（interconnect 互通；未互聯或調用失敗退化截斷式），以 `{"messages": [...]}` 改寫本步請求；LLM 請求溢出（`agent/request-error` 的 context_window_exceeded）時截斷式緊急壓縮並返回 `{"retry": true}` 讓宿主重試；逐 step 複用靠進程內 per-session 狀態指紋（歷史被改寫自動作廢重估；零文件 IO 對齊 DSH compaction-basic——持久化由宿主會話存儲承擔，文件落盤是加強版 dsc-billion-context 的領域），`DSC_COMPACTION_BASIC_CONTEXT_WINDOW`（默認 131072，0 禁用）與 `DSC_COMPACTION_BASIC_THRESHOLD` 可調；config.yaml `compaction: dsc-system`（或 `dsc-billion-context`）顯式選後端，宿主僅驗證 `Provides compaction` 能力聲明。

- **RPC 可靠性保障**：跨插件 gRPC 調用支持超時控制與指數退避重試機制；採用語義化版本範圍（`>=1.0, <2.0`）進行插件 API 兼容性檢查，允許補丁與次版本升級。

- **TUI 工具調用顯示**：所有工具名在 TUI 統一以 PascalCase 呈現（`skill` → `Skill`、`update_goal` → `UpdateGoal`、`shell` → `Shell`）；針對 `str_replace_editor`（或名稱包含 `editor` 的編輯器工具），會根據具體的 `command`（如 `view`, `create`, `str_replace`, `insert`）和 `path` 參數，顯示為 `StrReplaceEditor(View, /root/file/path)`、`StrReplaceEditor(Create, /root/file/path)` 等格式。工具結果的展示採用**結構化視圖聲明**（對齊 DSH 顯示契約）：實現工具的插件可選地在結果裏聲明**結構化視圖 spec**（`ExecuteToolResponse.view_json`，見 SDK 的 `dsc.CardView` / `dsc.TableView` / `dsc.PlainView`），TUI 以**單一渲染器統一繪製**三種版式——**card**（標題 + 語義色徽標 + 對齊鍵值字段）、**table**（對齊列頭 + 對齊行，超長單元格截斷）、**plain**（標題/徽標 + 正文塊），保證各工具結果風格一致。已內置插件的各工具均已按此實現專屬視圖（goal/todo/ask 卡片與表格、memory\_search/ssh\_list/web\_search 表格、web\_fetch/skill/lisp\_eval/ssh\_exec/shell 純文本、ssh/skill/browser 卡片等），宿主側 `run_code` 亦有專屬視圖（RunCode plain 塊，徽標為 stop\_reason，正文為返回值/錯誤）；宿主聚合 Tool 服務會把插件 `ViewJson` 與宿主工具視圖一併透傳到 `ExecuteToolResponse.view_json`。未聲明視圖的工具回退到通用 JSON 鍵值卡片（同層值列對齊），再退回到原文（`str_replace_editor` 的 diff 着色由通用兜底保留）。browser-use 插件支持 `DSC_BROWSER_CDP_URL` 指向既有 CDP 端點（跳過本地 chromium 啓動，供 mock chromium 集成測試等場景）。

- **多 Agent 工作流（workflow）**：宿主內置 `workflow` 模型工具（對齊 DSH tool-workflow）——模型編寫的 Lua 編排腳本，由 go-lua 的**協程排程器**執行，可扇出 subagent（`agent`/`parallel`/`pipeline`/`phase`/`log` 鈎子）；子代理默認無迭代上限，何時完成由模型自行決定；`return` 的 JSON 即結果；支持 `background: true` 後台運行。TUI `/jobs list | output <id> | kill <id>` 用户命令管理後台任務，模型亦可用 `job_output`/`job_list`/`job_kill` 工具。

- **程序化工具呈現（PTC）**：宿主內置 `run_code` 工具（對齊 DSH 的 PTC 概念）——模型寫一段**嚴格 Lua** 程序一把過組合多步工具調用，而不再逐個 call：程序裏每個可用工具以同名 Lua 函數呈現（`mytool{...}`），頂層 `return` 即結果；語言是帶類型註解、可空 `T?`、聯集、流式收窄的受檢方言（基於 go-lua）。`-mode ptc`（或 `DSC_PTC=1`）開啓**呈現模式**：把直接工具調用**摺疊**為唯一 `run_code`，其餘工具僅經其程序內 SDK 可調（對齊 DSH presentation；native/其餘模式下 `run_code` 對模型隱藏、也不可執行）；system prompt 引入 PTC 引導，`run_code` 描述攜帶「程序內可調工具」清單與嚴格 Lua 方言規範，助模型一把過組合多步。

- **項目級歷史隔離**：默認會話按當前工作區項目路徑命名（`C:\...\DeepClean` → `C--...-DeepClean.jsonl`），同項目跨時期共享歷史、不同項目隔離，不再使用硬編碼 `default.jsonl`；TUI 的當前會話標識與切換也統一對齊該項目 key（宿主 `DefaultSessionID()` 與 agent 的 `projectKey` 同源），`/session default` 解析到項目 key、`/export` 導出真實存檔，且**標題欄不再顯示會話 id**（經 `/sessions` 列表與 `/export` 管理，避免「顯示名 ≠ 存檔名」的脱節）；`/settings history <N|off|unlimited>` 實時生效並持久化到 config.yaml（`history_injection`：-1 禁止 / 0 未定義 / N>0 啓用 N 條）。

- **沙箱範圍可見**：TUI 左下角狀態欄隨 `/sandbox` 即時顯示當前工作範圍——各檔均顯示工作區目錄基礎名（限長），沙箱模式經後綴標記呈現（🔒 只讀 / ✎ 工作區寫 / ⚡ 全開）。

- **事件體系（對齊 DSH harness 事件）**：宿主 EventBus 採與 DSH cordis events 一致的五種分發模式（`emit` 廣播通知 / `waterfall` 洋葱攔截 / `serial` 順序 / `bail` 短路 / `parallel` 並發），並經互通機制把宿主事件廣播給插件（`Hook.OnEvent`），**不限定插件類型**——任意註冊了 Hook 的插件（tool/dsc/llm/agent/policy）都能訂閲，對齊 DSH cordis 的「事件廣播類型無關」；令插件可獨立訂閲系統事件而不改宿主。已對齊的關鍵事件：工具流水線 `tools/pre-execute` / `tools/execute` / `tools/post-execute`（waterfall 攔截，veto 即阻止；execute 供插件包圍執行與超時策略）與 `tools/result`（emit 結果廣播）；agent 回合生命週期 `agent/status`（running/idle）與 `agent/error`（emit，成功/失敗區分）。因 DSC 的 agent 為獨立 gRPC 插件進程（不同於 DSH 宿主內循環），僅對齊機制與有真實消費者的事件，不機械照搬無消費者或需跨進程空轉的事件。這些領域事件與運行時日誌可經管理 API 的 SSE 端點實時觀測：`/plugins/domain-events`（推送全部 EventBus 領域事件，含字段保真載荷）與 `/plugins/logs`（推送宿主日誌與插件子進程經轉發上來的日誌；宿主與插件 logger 統一接入扇出 sink，即使默認靜默模式也按需可察）。管理 API 另提供 `GET /plugins/tools`，一次性返回模型當前可直接調用的工具目錄（含插件熱加載的動態工具，如 tool-lua-host 腳本工具），作為調試視圖。`-input` 非 headless 且已加載通知插件（如 notify）時，回合結束後會短暫寬限（約 0.8s）再關閉插件，確保異步的回合完成音效（約 0.29s）能完整播完，避免被插件進程回收截斷。

- **插件安裝/管理（模型可自助）**：宿主內置六個模型工具，讓模型動態管理插件——安裝 / 升級 / 卸載 / 列出 / 運行期載入 / 運行期卸載，詳見「[模型自助動態插件管理](#模型自助動態插件管理)」一節。

- **CRON 工具（模型可調用）**：宿主內置 `cron_add` / `cron_list` / `cron_remove` / `cron_set_enabled` 四個模型工具，把宿主側的 cron 定時任務調度器以工具形式暴露給 agent，讓模型在評測（如 `tool-agentic-bench` 的 CRON 案例用例）或日常會話中能真實調用 DSC 的 CRON 機制（增 / 刪 / 列 / 啟停），而非僅經 TUI 斜杆命令或管理 API。變更類工具（`cron_add` / `cron_remove` / `cron_set_enabled`）聲明 `ApprovalRequester`，在審批策略非 `never` 時會被前置門控攔截；`cron_list` 是隻讀工具、無需審批。

- **管理 API 作為「管理能力」（按需啟動，對齊 DSH 能力邊界模型）**：管理 API（admin server）本質是一種「管理能力」——只有需要它的插件（如 `tool-harness-webui`）才經 `Requires: [{Type:"dsc", Capability:"admin"}]` 聲明依賴。宿主按以下優先級決定是否啟動管理 API：1. 用户顯式 `-admin <addr>` → 強制啟動；2. `DSC_ADMIN_ADDR` 環境變量 → 強制啟動；3. 配置中有插件聲明 `requires/dsc/admin` 能力依賴 → 自動啟動；4. `DSC_NO_ADMIN=1` → 強制關閉；5. 默認 → 關閉（TUI 終端用户通常不需要管理 API，避免不必要的端口綁定與安全暴露面）。啟動後實際監聽地址經 `os.Setenv("DSC_ADMIN_ADDR", actualAddr)` 注入插件子進程環境，供 webui 等插件讀取。端口衝突時自動自增（如 9999 → 10000 → 10001…，最多 100 次）。

- **聲明式插件依賴（按能力匹配，對齊 DSH/Cordis 的 provide + inject 模型）**：插件作者可經 `dsc.Config.Requires` 顯式聲明本插件依賴「某項能力」（capability），經 `dsc.Config.Provides` 聲明本插件提供哪些能力——對齊 DSH/Cordis 的 `inject` / `provide` 雙向聲明機制。宿主在安裝 / 加載插件時掃描已加載插件的 `PluginInfo.Capabilities` 普通能力鍵（如 `supports_images: "true"`、`filesystem: "true"`），找到聲明該能力的插件並建立依賴關係——避免用户在 config.yaml 手工指定 `depends_on` 按插件名引用易出錯。解析得到的依賴關係存於宿主運行時態 `m.resolvedDeps`，供運行時態查詢與反應式重算使用；`config.yaml` 不再含 `depends_on` 字段——能力依賴由插件二進制內的 `sdk.Config.Requires` 自描述，無需落盤。詳見 SDK README「聲明式依賴（Config.Requires）」一節。

- **能力契約校驗（fail-loud，對齊 DSH 同域唯一 provider 約束）**：對齊 DSH/Cordis 的 `reflect.ts` `provide()` 在同一 scope 內第二個註冊同一服務名時 throw 的語義——DSC 的 `findProviderByCapabilityLocked` 對非 LLM 類型能力找到多個 provider 時 **fail-loud** 返回 error，不再靜默取首個，要求用户在 config.yaml 中只啟用一個或經顯式選擇消除歧義。LLM 類型除外——對齊 DSH 的 `LlmRuntime.registerAdapter` adapter registry 模型，多個 LLM adapter 可註冊不同 provider route 並存。

- **多 LLM provider 顯式選擇（對齊 DSH agentDefaultModel）**：當多個 LLM 插件並存時（如同時啟用 `llm-openai` 與 `llm-anthropic`），宿主優先選擇 `config.yaml` 的 `default_llm` 字段指定的 LLM 作為 agent 的 primary provider——對齊 DSH 的 `agentDefaultModel.currentSelection()` 顯式選擇機制。若 `default_llm` 未配置，按名升序取首個並記 warn 提示用户設 `default_llm` 消除歧義。所有 LLM 插件默認提供 `CapabilityLLM="llm"` 能力（由 `llmMetadataServer` / `llmSdkMetadataServer` 自動聲明），agent 經 `Requires: [{Type:"llm", Capability:"llm"}]` 聲明依賴。

- **配置自癒**：每個成功啟動後，把已生效的 `config.yaml` 與當前 mode 的 preset（如 `standard.yaml`）**各自獨立備份**到源文件同目錄的備份子目錄（`config.yaml` → `config-backups/`、preset → `preset-backups/`；旋轉保留最近 10 份、按各自前綴區分、互不串擾）。當某份配置因改壞或壞插件導致啟動報錯時，宿主會先把壞版各自留檔，再**分別還原各自最近正常備份**、重建插件集重試一次並以降級模式繼續啟動——而非直接退出，避免「模型搞壞配置就再也起不來」。同時 config.yaml 中啟用的 tool/policy/dsc 插件正式併入啟動合併集（與 preset 按名去重、**preset 優先**——preset 屬具體的預設，同名衝突取 preset，config 僅補 preset 沒有的，使模型安裝的插件仍能跨重啟生效）。

- **插件目錄自癒**：維持 `plugins/` 的「上次正常」快照（兄弟目錄 `plugins-backup/`，二進制大故僅當有新內容才刷新）。當插件目錄與配置無法對齊（如插件二進制缺失/損壞）導致啟動加載失敗時，從快照回拷合併恢復（**容錯拷貝**：運行中的插件 `.exe` 被進程鎖住會跳過——本就正常；真正缺失/損壞、未運行的二進制會被回拷）後再續啟。恢復後還會盤點「未被當前配置引用的孤立插件目錄」並**僅告警、不刪除**——此類插件從未啟用，無法判斷其可用性、亦不能替用户保證將來不用，故先保留；日後若用户啟用其卻導致啟動加載失敗，再由本機制兜底處理。

## 模型自助動態插件管理

宿主內置六個模型工具，讓模型可以自助地安裝、升級、卸載、列出，以及在**運行期**載入/卸載 DSC 自身插件——即本機二進制 Go 程序、經 `dsc-sdk` 構建、go-plugin/gRPC 加載的插件（有別於 Go 標準庫的 `plugin` 包；對齊 SKILL 安裝，經聚合 Tool 服務暴露給模型）。插件**用與不用由配置決定**，與是否被構建解耦；這些工具在運行時動態生效、無需重啟。

### 命名約定與安全保障

- **命名約定**：插件目錄 `plugins/<type>-<name>/`、執行檔 `<type>-<name><ext>`（Windows 下 `ext=.exe`）、`type`∈tool/llm/agent/policy/dsc、`name` 僅 `[A-Za-z0-9_-]`。

- **寫 config 前備份**：任何改動 config.yaml 前先備份（`config.yaml.<ts>.bak`），防止模型寫壞配置。

- **幹跑=live 加載**：安裝先真實 live 加載插件、驗證類型/元數據一致才落盤，失敗則回滾（刪除已拷貝目錄、config 未寫入）。

### 工具一覽

| 工具                     | 用途    | 關鍵行為                                                                    |
| ---------------------- | ----- | ----------------------------------------------------------------------- |
| `install_dsc_plugin`   | 安裝插件  | 拷貝到 `plugins/<type>-<name>/`，live 加載校驗後落盤 config，失敗回滾                   |
| `upgrade_dsc_plugin`   | 升級插件  | 部署版本化二進制 `<name>-v<版本><ext>` 並觸發宿主熱更替；失敗保留舊實例、新文件供重啟兜底                  |
| `uninstall_dsc_plugin` | 卸載插件  | 從 config 移除條目，可選刪除 `plugins/<name>/` 目錄                                 |
| `list_dsc_plugins`     | 列出插件  | 合併 config 聲明 + 運行態 + 磁盤三源，逐條標註 `state`（loaded=/configured=/orphan=磁盤孤兒) |
| `load_dsc_plugin`      | 運行期載入 | 載入「已存在於 plugins/ 但未在 config 聲明」的插件；默認僅當前進程生效，可選持久化                      |
| `unload_dsc_plugin`    | 運行期卸載 | 停止本進程服務並註銷其工具；可選從 config 移除條目                                           |

安裝/升級/卸載後插件即時熱加載、不需重啟；模型可據「能否加載 ACTIVE」判斷安裝是否正確。

### 運行期載入 / 卸載（load / unload）

`load_dsc_plugin` 用於**運行期載入「已存在於 plugins/ 目錄、但未在 config 聲明」的插件**（如目錄裡有但仍處孤立未啟用者）：模型直接傳插件 id（`plugins/` 下目錄名，如 `tool-musicplayer`）即載入，其工具本會話立即可用。走宿主原生聚合 Tool RPC 調 `LoadPlugin`，**默認不寫 config.yaml**：

- `persist=false`（默認）：僅對當前進程生效、不改配置；重啟後按需重新 load。

- `persist=true`：先備份再顯式寫回 config.yaml，重啟後仍自動加載。

- 安全：僅允許 `pluginsRoot` 內可執行檔 + 嚴格命名 + 已載入冪等返回（重複 load 不報錯）。

對稱的 `unload_dsc_plugin` 在**運行期卸載已載入插件**並註銷其工具：

- `persist=false`：僅本進程停止、不改 config。

- `persist=true`：同時從 config.yaml 移除該條目（先備份）；`plugins/` 目錄文件一律保留。

- 安全：未載入時卸載也冪等處理，可按需僅清除 config 條目。

`unload_dsc_plugin` 用以對 `load_dsc_plugin` 的臨時/持久載入做對稱清理；完整卸載（移除 config 且可選刪目錄）則用 `uninstall_dsc_plugin`。

## 與 DSH（DeepSeek Harness）的對比

DSC 與 DSH 同源於「一切皆插件」的設計哲學，兩者在概念層高度同構，但語言棧與運行形態不同：DSH 為 TypeScript / Node.js（cordis 插件框架），DSC 為 Go / go-plugin + gRPC。

### 核心功能對比

| 功能領域           | DSH（deepseek-harness）                                                            | DSC                                                                                                                                                                                                                    |
| -------------- | -------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 宿主/插件架構        | cordis 插件框架，一切皆插件                                                                | go-plugin + gRPC，一切皆插件，支持熱插拔/熱重載                                                                                                                                                                                       |
| 沙箱隔離           | 內核級：bwrap（bind mount）/ Landlock（`sandbox-local` + 各 runner profile）              | 宿主工具級攔截（Windows 兼容）：工具流水線 pre-execute 瀑布，三檔 `read-only / workspace-write / full-access`                                                                                                                                |
| 沙箱策略歸屬         | `ctx.sandboxPolicy` 單一歸屬（mode + workspace 根）；`renderPolicyContext` 以真實路徑呈現給模型    | 宿主 `Manager.sandboxPolicyVal` 單一歸屬；TUI `/sandbox` 即時切換；system prompt 注入 `sandbox:policy` 上下文（同樣以真實根路徑呈現）                                                                                                               |
| 路徑圍欄           | `fs-sandbox` containment：詞法快速路徑（Windows 忽略大小寫）+ 文件身份（dev/ino）回退（識別 8.3 短名、大小寫別名） | `inWorkspace`：`CanonicalPath` 解析真實路徑（Windows 用 `GetFinalPathNameByHandle` 穿透 junction/symlink，Unix 用 EvalSymlinks）再做包含判定，防 workspace 內指向外部的連結寫穿；`/workspace` 虛擬前綴映射到統一根（sandbox 與 shell 工具、str-replace-editor 共用該別名語義） |
| 會話             | 事件日誌（event log）+ `deriveMessages()` 派生模型歷史                                       | 事件溯源 `session` 包 + `DeriveMessages`（同構）                                                                                                                                                                                |
| 上下文壓縮          | `compaction-basic`：thresholdRatio / retainRatio                                  | `dsc-system` 駐留壓縮（自宿主遷入，宿主零壓縮代碼）：80% 閾值接管（後端標記驅動，agent 內聯壓縮自動跳過）、16% 尾部保留（≥1024 token）、聚合 LLM 摘要（未互聯退化截斷式）、per-session 狀態指紋複用、溢出緊急壓縮重試                                                                                                                                                                              |
| token 計量       | TokenMeter：本地精確 tokenizer，缺省字符估算回退                                               | 以服務端上報 usage（精確）為準；服務端不可用（重啟）或低估（提示緩存命中）時回退字節級啓發式估算 + 提示緩存感知（`input_tokens + cache_read_input_tokens`）                                                                                                                 |
| 歷史注入           | 無按條數限制機制（靠壓縮限界）；`maxMessages` 僅用於歷史查看分頁                                          | **`/settings history N\|off\|unlimited`** 項目級會話隔離（按工作區命名）+ 持久化到配置（`history_injection` 三態）                                                                                                                              |
| 技能             | `skill/` provider registry + catalog/loader tool（`ctx.skills`）                   | `skills/builtin` + `skills/installed`，`skill` 按需加載、`install_skill`/`uninstall_skill` 管理                                                                                                                           |
| plan/goal/todo | plan-mode、goal、todo 領域                                                           | 對齊：宿主託管 plan/goal/todo 工具，狀態經事件日誌折疊                                                                                                                                                                                    |
| UI             | Web UI（`apps/web`）                                                               | TUI（bubbletea）+ Web UI（`tool-harness-webui`）                                                                                                                                                                           |

**PTC（程序化工具呈現）差異**：兩者概念同構，且呈現方式現也已對齊——模型寫一段程序組合多步工具調用、一把過執行；`ptc` 下把直接工具調用**摺疊為唯一** **`run_code`**（其餘工具僅經其程序內 SDK 可調），native 模式則對模型**隱藏** **`run_code`、也不可執行**（run\_code-only 的高隱藏）。僅實現語言不同：DSH 的原生 PTC 用 **TypeScript**（runtime 本身 TS/Node）；DSC 用 **嚴格 Lua**（go-lua，帶類型註解、可空 `T?`、聯集、流式收窄的受檢方言）。

### 功能對齊程度

- **完全對齊（概念同構）**：事件溯源會話、審批策略按會話（`/approval` 設當前會話，缺省 ask；策略亦寫入會話日誌 `approval/policy`，resume/fork 後摺疊恢復，並隨每次工具調用由 agent 轉發——宿主重啟後 per-session 策略即恢復、無需重設）、**沙箱↔審批 preset 綁定**（`danger-full-access→never`、`read-only/workspace-write→ask`，未顯式設置時生效；顯式 `DSC_APPROVAL`/`/approval`/會話覆蓋優先，對齊 DSH permission-presets）、沙箱三檔策略語義、沙箱升級審批（`approveEscalation`：被拒操作攜 `sandbox_permissions`+`justification` 升級重試 → `ask` 審人 / `never` 自動拒，非嚴格加寬執行前拒絕；`sandbox_permissions`/`justification` 已發佈入沙箱約束工具族 (`shell`/`str_replace_editor`) 的參數 schema，模型可發現性同構）、工具聲明審批（`ApprovalRequester`，對齊 DSH tools `kind:'ask'`/`serviceAsk` 入口②；DSC 插件管理變更類工具 `install/load/unload/uninstall/upgrade_dsc_plugin` 已接入，`list` 只讀不審）、審批提問接入取消信號、升級提示 subject 按工具族（shell→`command` / 其餘→`operation`）、審批審計寫入會話日誌（`approval/asked`+`approval/decided`）、上下文壓縮（pre-step 壓力檢查 + 尾部保留）、plan/goal/todo 領域、以真實路徑呈現 workspace 根給模型、`approval:policy` 運行時上下文（反映當前會話真實策略）、技能注入。

- **部分對齊（同概念、異實現）**：沙箱從 DSH 的內核級（bwrap/Landlock）改為 DSC 的宿主工具級攔截（換取 Windows 兼容與可移植性，代價是「策略圍欄」而非「內核邊界」）；雖為工具級，但對已知寫路徑、Windows junction/symlink 穿越、以及不可定位寫路徑的解釋器逃逸，均已於工具流水線 pre-execute 階段 fail-closed 封堵（見下方「各自獨特實現」）；token 計量從 DSH 的 TokenMeter（本地精確 tokenizer）改為 DSC 的「服務端 usage + 字節級啓發式估算回退」；技能注入從 DSH 的 provider registry 改為目錄掃描 + `ListContext` 索引。

- **日誌跟蹤（對齊 DSH 觀測體系）**：兩層分工同構於 DSH——①**規範會話日誌**（`sessions/` JSONL，事件溯源、常駐落盤）承載診斷事件：`llm/attempt`（log-only，格式 v3 新增，對齊 DSH `llm/*`+`assistant/attempt` 的診斷定位）每次 LLM 調用結算必落一條——finish_reason、token 用量、耗時、錯誤文本與穩定錯誤碼（`context_window_exceeded`/`rate_limited`/`network_error`/`unknown`）、部分內容長度，截斷/provider 報錯/流中斷/上下文溢出/耗時一眼可查，無需審計代碼；回合閉合不變量（turn/end 恆以 completed/goal-concluded/max-iterations/error/cancelled 收口，step/start/end 恆配對），導出記錄不再出現懸空回合；②**宿主運行日誌**（`-log` 開啟，默認靜默零噪音）：hclog 分級（`DSC_LOG_LEVEL` 可調）全鏈路埋點——每次 LLM 嘗試（含 provider 切換與重試）、每次工具執行（計時與成敗）、workflow 生命週期、插件裝載、會話操作；插件子進程 stderr 經 go-plugin 轉發匯入同一日誌流，默認靜默時亦可經 ADMIN `/plugins/logs` SSE 按需觀察。

- **DSC 擴展（DSH 沒有）**：`/settings history` 歷史注入條數限制（DSH 僅靠壓縮限界）+ 項目級會話隔離 + 配置持久化；提示緩存感知的容量計算（本地 llama.cpp 緩存命中時 `input_tokens` 僅含新增部分，須加回 `cache_read`）；`/sandbox` TUI 即時切換；`/approval` TUI 即時切換審批（`DSC_APPROVAL` 可配默認 ask/never）；多會話 TUI 管理（`/session new\|list\|switch\|delete`）；cron 定時任務；多 Agent workflow 後台運行（`background: true`）+ TUI `/jobs` 管理命令；`-input` 自動化多輪入口；`-headless` 精簡單發模式；`-debugger` 管理 API 觀察端點；管理 API 的 `/plugins/domain-events` 與 `/plugins/logs` SSE 流實時觀測領域事件與宿主/插件日誌。

### 各自獨特實現

- **DSH 獨特**：內核級沙箱（真實 OS 邊界，不可信代碼經 `ctx.shell` 隔離）；跨能力族統一的可寫根集合（`writableRoots` 與 Seatbelt profile 共享，防止 fs 圍欄與 runner 漂移）；文件身份（dev/ino）圍欄回退；TokenMeter 精確計量；API 代理層（`api-proxy`：歷史分頁、子代理、投影）。

- **DSC 獨特**：純 Go + go-plugin/gRPC 全棧；TUI 交互（拖選複製、流式期間滾動、狀態行「N 輪 M 步 · 每秒 X 詞元 · 初速 Y 詞元 · 已用 N% · 緩存命中 N%」實時指標帶，速率語義對齊 DSH turn-metrics：每秒=解碼吞吐、初速=含首響等待的起步速率，兩讀數差距即 TTFT 體感）；Windows 兼容的工具級沙箱攔截（pre-execute 三檔策略 + junction 穿越與解釋器逃逸的 fail-closed 封堵）；外部腳本鈎子（`-hooks`：嚴格 LUA 由本地 go-lua 進程內解釋或原生可執行文件直接 exec，BeforeTool/AfterTool 可 veto/改寫，對齊 DSH hooks 橋接且規避視窗腳本引擎碎片問題）；Windows 全部子進程（插件、LSP 服務器、外部鈎子、shell 外部命令、PDF 渲染器等）以 `CREATE_NO_WINDOW` 隱藏控制枱啟動，杜絕 TUI 中彈出新終端窗口；**宿主存活看門狗**（宿主經 `DSC_HOST_PID` env 注入自身 PID，SDK 在 `Serve` 前啟動看門狗 goroutine 定時經內核級 PID 檢測——POSIX Signal(0)/Windows GetExitCodeProcess——宿主是否存活，連續 3 次（間隔 5s）檢測失敗即 `os.Exit(0)` 主動退出，補上 go-plugin 原生機制缺失的「插件→宿主」存活檢測方向：宿主崩潰/OOM/被強殺時插件不會殘留為孤兒進程；對齊 DSH Node.js 子進程的父進程 PID 監控 `process.ppid + process.kill(ppid, 0)`）；提示緩存感知的容量與壓縮判定；`/settings history` 歷史注入控制；事件溯源多會話 + `/session` 管理；cron 調度；`-debugger` 管理 API；`-input` 重定向多輪自動化；`-headless` 精簡單發（仿 DSH harness headless）。

## 支持的插件

### LLM 插件

- `llm-openai`（OpenAI 兼容端點：DeepSeek API / llama.cpp server 等。**輸出上限=有效上下文窗口**：宿主一律注入 `DSC_MAX_OUTPUT_TOKENS`=有效上下文窗口值——探測命中 LLAMACPP 家族端點（`/v1/models` 返回 `meta.n_ctx`）取探測窗口值，探測不到（雲端）取配置 `context_window` 值（不正確可隨時再設），不分本地/雲端、行為一致；`OPENAI_MAX_OUTPUT_TOKENS` 顯式配置優先（>0 隨請求攜帶，顯式 0=不攜帶）；壓縮等請求級參數優先於以上兩者）

- `llm-anthropic`（Anthropic 兼容端點：DeepSeek anthropic / llama.cpp server 等。**輸出上限=有效上下文窗口**：宿主一律注入 `DSC_MAX_OUTPUT_TOKENS`=有效上下文窗口值——探測命中 LLAMACPP 家族端點（`/v1/models` 返回 `meta.n_ctx`，與 anthropic 口同端口）取探測窗口值，探測不到（雲端）取配置 `context_window` 值（不正確可隨時再設），不分本地/雲端、行為一致；anthropic 協議把 max_tokens 視為 required，顯式攜帶窗口值在 llama.cpp 側受上下文自然鉗制、無害，並對抗其對缺席值自填的保守默認（laamaafung 為 4096）；`ANTHROPIC_MAX_OUTPUT_TOKENS` 顯式配置優先（SDK 無 omitempty，零值字段由請求中間件摘除，不會以 `"max_tokens":0` 上送）；壓縮等請求級參數優先於以上兩者。思維鏈 stderr 調試打印默認關閉——reasoning 本就隨流式幀送宿主/TUI，需排查插件本身時設 `DSC_LLM_DEBUG` 才輸出）

- `llm-ollama`（Ollama 兼容端點：本地 Ollama server。與 `llm-openai`/`llm-anthropic` 同為 LLM provider 插件，經 Ollama Go SDK 直連 `/api/chat`；支持 reasoning/thinking 模型（默認開啓，`OLLAMA_THINKING=0` 可關閉），從響應 `Message.Thinking` 提取思維鏈幀渲染到 TUI；@文本附件引用（`dsc-txt://`）解析為文本串前綴拼入 user 消息 Content）

#### 圖像輸入（視覺）

`llm-openai` 與 `llm-anthropic` 支持把本地圖片作為視覺輸入傳給視覺模型（如
`deepseek-v4-flash-vision-exp`；llamacpp server 的 OpenAI/Anthropic 兼容端點同樣
接受該格式）：

- 圖像以**內容尋址引用**隨會話歷史保存（用户消息 `dsc-img://<sha256>`；工具結果
  截圖 `dsc-shot://<sha256>`），不隨歷史膨脹；上下文窗口內後續輪次模型仍可見；

- 圖像字節按生命週期分庫：用户 `@` 引用的圖片寫入持久附件庫（可執行目錄下
  `attachments/<sha256>`，文件名只取內容哈希不帶後綴，對齊 DSH 與 `sessions/`
  等目錄舊例——同內容無論聲明/改寫什麼擴展名都落同一文件）；工具產生的操作截圖
  （computer-use 等）由宿主入庫口統一折算為 `dsc-shot://` 引用，字節寫入
  `temp/screenshots/`，**24 小時後隨 temp/ 目錄清理過期**——操作截圖只有短期
  觀察價值，不進持久附件庫、不寫用户 workspace，插件回傳的 data URL 絕不進入
  會話日誌；

- LLM 請求時把引用投影為 base64 嵌入（OpenAI 端點為 `image_url` 塊，Anthropic
  端點為 `image` 塊）：最長邊超過路由上限（1568，對齊 Anthropic 視覺最優分辨率）
  時等比降採樣——不透明圖重編碼為 JPEG、帶透明度保留 PNG；未超限原樣透傳；
  引用失效（截圖過期/附件缺失）降級為穩定佔位文本，模型由此知道該處曾有圖；

- 請求面圖像預算卸載（對齊 DSH RequestImageOffloadPolicy，`dsc-system` 駐留
  image-offload，agent/pre-step 槽）：歷史中的圖像引用數超過單請求上限（默認 12，
  `DSC_MAX_REQUEST_IMAGES` 覆蓋，`0`=不限制）時按最舊優先退役為佔位文本——最新
  觀察永遠在場，token 與費用不隨 CU 截圖輪次線性膨脹；卸載是純瞬態投影，會話
  日誌原樣保留（自 agent-react-loop 抽取遷入：策略歸還插件，agent 請求組裝零預處理）；

- 單圖超過約 20 MiB 且端點指向 DeepSeek 時自動上傳 Files API（`purpose=user_data`）
  並以 `file_id` 引用（Anthropic 端點自動附帶 `anthropic-beta: files-api-2025-04-14` 頭）；
  llama.cpp 等本地 server 無 Files API，始終內聯；

- 圖像輸入**默認按模型能力自動判斷**：請求 `/models` 讀取模型的 `input_modalities`
  （上報含 `image` 則啓用；未上報/未知默認放行，對齊 DSH 僅按模型能力校驗）；
  `DSC_NO_VISION=1` 可強制關閉（自動判斷失靈時的逃生口）；
  附件庫根目錄可用 `DSC_ATTACHMENT_DIR` 覆蓋（缺省 `<ExecDir>/attachments`）。

TUI 輸入中以 `@文件路徑` 引用本地文件（支持 `/workspace` 虛擬根別名與絕對路徑），
與圖像讀取方式對齊：圖片文件（`dsc-img://`）作為多模態 image 塊隨本輪（或運行中
注入）發送給模型；**文本文件**（`dsc-txt://`，含 NUL 嗅探判二進制、約 1 MiB 上限）
則把文件內容作為文本塊注入請求，供模型直接讀取文字而無需用 shell/編輯器再去打開；
二者 `@` 引用的文字本身仍作為提示傳給模型。

TUI 輸入框按 `@` 會彈出當前工作區的文件候選篩選列表（對齊 REX：目錄優先、可下鑽，
`↑/↓` 選擇、`Tab`/`Enter` 補全、`Esc` 關閉），隨輸入過濾並在選中後以
`@some/path/file.format` 形式填入；含空格的路徑以反斜杆轉義保持單一引用 token。

### Agent 插件

- `agent-react-loop`（ReAct 主循環：流式消費聚合 LLM、執行聚合工具、事件溯源會話。**LLM 調用全程留痕**：每次調用結算落 `llm/attempt`（log-only）——finish_reason/用量/耗時/錯誤與穩定錯誤碼成敗皆錄；**回合閉合不變量**：turn/end 恆以某 reason 收口（異常/取消由 defer 兜底），杜絕懸空回合。**輸出截斷防護**：檢測 `finish_reason=max_tokens`/`length`——純文本被截斷時向 TUI 告警（不自動續行：截斷根因已從 LLM 插件源頭移除，續行行為待「中斷」根因經真機觀測徹底確認後再引入，避免掩蓋問題）；截斷響應攜帶的工具調用若參數 JSON 殘缺則拒絕執行，落合成 tool/result 保持 tool_use/tool_result 配對並請模型重發，消除「以空參/殘參下發工具觸發報錯」的頑疾。TODO 追問（**不設預算**：清單非空就持續追問，驅動模型逐項處理完——完成或取消，直到清單清空才收輪；連續追問達 5 次升級 Warn 留痕防靜默空轉）、goal round、重複調用提醒等續行驅動齊備）

### Tool 插件

- `tool-filesystem`（shell：mvdan POSIX 解釋器，默認以 `DSC_WORKSPACE_ROOT` 為工作目錄，每次調用不傳 `session_id` 時為**全新會話**——cwd/變量/函數不跨調用保留，跨調用切目錄應傳 `workdir` 而非依賴 `cd`（與 DSH 最新 tool-bash 同語義顯式告知模型），傳一致 `session_id` 則跨調用維持 cwd 與環境變量；在 AST 層把模型傳入的 `/workspace` 虛擬根前綴映射到真實工作區根——`cd /workspace`、`ls /workspace/x` 等初期探索不再報 no such file or directory，路徑統一正斜杆；僅當 `/workspace` 後緊跟分隔符（`/` 或 `\`）或處於路徑結尾時，才按其映射為工作區根，`/workspacefoo` 之類的路徑不會誤當作工作區根別名——該語義與 sandbox 的 `/workspace` 別名判定一致。常用工具 `mkdir`/`ls`/`cat`/`touch`/`rm`/`cp`/`mv`/`grep`/`head`/`tail`/`wc` 已**進程內實現**（`interp.ExecHandler` 攔截，純 Go 無外部依賴），因此即便在 Windows 且插件子進程 `PATH` 被宿主過濾時這些命令仍可用；未命中的命令仍回退默認 `PATH` 查找外部程序（Windows 上以 `CREATE_NO_WINDOW` 隱藏控制枱啓動，避免 TUI 中彈出終端窗口）；提供 `filesystem` 能力；`tree` 亦為**進程內實現**（`-L` 層數 / `-I` 排除 / `-a` 隱藏項 / `-d` 僅目錄，目錄優先按名升序）——消除 Windows 上 PATH 命中 `C:\Windows\tree.com` 的回退（不支持 `-L/-I` 且輸出為系統碼頁字節）；`ls` 支持 `-F` 分類符（`/`、`*`、`@` 等）、`-t` 按時間、`-S` 按大小、`-r` 逆序及 `-laF` 組合短選項；Windows 上**裸 POSIX 絕對路徑**（`/`、`/x`）保持真實根語義（與 Linux 一致，經 `filepath.Abs` 解析為當前盤根），不再錨定工作區根——引用工作區文件須顯式 `/workspace` 前綴或盤符路徑，`/dev/null` 保持原樣（mvdan 特判重定向到 NUL）、`//` UNC 路徑不改寫、`/mnt/<盤符>` 映射到對應盤（僅 Windows）；shell 命令輸出經 **UTF-8 淨化**後回傳——中文 Windows 原生命令的 OEM/ANSI 碼頁輸出（如 GBK）先嚴格解碼還原為正確中文，無法解碼的字節退化為 U+FFFD，杜絕 `grpc: error while marshaling: string field contains invalid UTF-8` 整個工具結果被拒發）

- `tool-pdf`（PDF 讀取與創建。讀取側：`pdf_read_text` / `pdf_extract_tables`（列對齊表格網格輸出）/ `pdf_info` / `pdf_outline` / `pdf_search` / `pdf_extract_images`，自寫內容流解釋器 + 字體解碼（WinAnsi/MacRoman/Standard + ToUnicode CMap；CID 字體缺 ToUnicode 時回退解析內嵌 TrueType cmap 解碼中文），按 y 聚合行、按橫向間隙識別多欄（以製表符分隔）、依 `Tm` 旋轉角把非橫行單獨分區輸出，盡力還原閲讀順序。創建/整理側：`pdf_create_text` / `pdf_images_to_pdf` / `pdf_append_text` / `pdf_merge_pdfs` / `pdf_split_pdfs` / `pdf_extract_pages`，支持標準 14 字體與自帶 CJK 字體（Type0 嵌入子集、字符級折行與真實行距分頁）。攜帶中文字體體積大不進公開倉庫，需按插件 `fonts/字体下载.txt` 自行下載；go.mod module 名為 `tool-pdf`（與目錄/二進制命名一致，直接 `go build` 產物即合規）。提供 `pdf` 能力）

- `tool-str-replace-editor`（文件編輯：接受 `/workspace` 虛擬根前綴並剝離映射到工作區根；提供 `editor` 能力）

- `tool-browser-use`（無頭瀏覽器工具：`web_fetch` / `web_search` / `browser_click` / `browser_type` / `browser_screenshot`；提供 `browser` 能力）

- `tool-computer-use`（桌面觀察與操作：`computer_use_screen` 截圖隨工具結果回傳視覺模型（經宿主入庫為 `dsc-shot://` 引用，temp 24 小時生命週期，不落用户 workspace） + `computer_use_click/move/drag/scroll/type/key/paste/cursor_position/screen_size/check`；基於 robotgo，座標紀律=截圖即座標系；Linux 需 X11 開發頭文件（libx11-dev/libxtst-dev/libxi-dev）與 DISPLAY，僅本機平台隨發佈包（CGO 插件不交叉編譯）；提供 `computer-use` 能力）

- `tool-lisp-eval`（Lisp/Scheme 精確有理數求值：`+ - * /` 變參精確運算、`3/4` 分數字面量、任意精度整數；浮點走 `f+ f- f* f/` 逃生艙；提供 `lisp-eval` 能力）


- `tool-lua-host`（LUA 腳本宿主：腳本註冊工具，宿主互通複用 LLM/Tool/Notify；內置只讀 `list_lua_tools` 枚舉當前已註冊的 LUA 腳本工具；腳本註冊的工具以**腳本名為前綴**暴露（腳本 `example` 的 `ping` → `example_ping`，故模型從工具名即知它出自哪個腳本，`dsc.script.name` 供腳本拼自身工具名）；腳本工具在創造模式下熱加載（約 2s 輪詢掃描 `scripts/`），宿主會節流同步其到模型可直接調用的工具目錄——新腳本工具無需重啓即可被模型直接調用）

- `tool-sql-host`（.dsp 插件宿主：.dsp 是「dsc's plugin」，實質是一個 SQLite 庫——插件的元數據（`dsp_meta`）、代碼（`dsp_blobs`）、運行期狀態（`dsp_state`）同處一庫，即「程序即數據」；宿主打開後取出其中的 LUA 代碼執行並把註冊的工具以**插件名為前綴**暴露（插件 `hello` 的 `greet` → `hello_greet`，與 `list_dsp_plugins` 列出的插件名逐字對應、歸屬一眼可查；內置只讀 `list_dsp_plugins` 概覽已加載插件），插件經 `dsc.sql.*` 對自己那個庫做 SQL、經 `dsc.store.*` 讀寫自身狀態（宿主重啓後仍在，因為狀態就在它自己的文件裏）；後綴強制校驗 + SQLite 魔數 + `application_id=0x44535031`（"DSP1"）四道門禁自證身份；腳本側 SQL 加保守守衞（禁 ATTACH/DETACH、禁改元數據與代碼表、禁運行期 DDL，自有表由打包期 `schema.sql` 落庫）；創造模式下按**產物哈希**（只覆蓋元數據與代碼，狀態寫入不觸發）熱加載，並可用 `pack_dsp` 把源目錄打包成 .dsp；參考實現來自 SelfDB 的「可執行文件即數據庫」思路，但去掉了內核層——不需要可執行位與 binfmt_misc，執行主體是 Go 宿主進程）

- `tool-memory-service`（記憶庫工具：完整增刪改查——`memory_search` 檢索（FTS5 + LIKE，時間衰減排序）、`memory_add` 新增（內容去重）、`memory_delete` 按 ID 刪除、`memory_update` 按 ID 修改、`memory_list` 分頁列表；AfterTool 自動記憶鈎子把其他工具成功執行結果寫入記憶庫，落點宿主可執行目錄 `memory/`，跨會話共享；提供 `memory` 能力）

- `tool-harness-webui`（獨立 HTTP 服務，代理宿主 admin API 的前端）

- `tool-ssh`（SSH 遠程命令執行終端：`ssh_connect` / `ssh_exec` / `ssh_list` / `ssh_close` 四類持久會話，登錄支持密碼或私鑰，會話按 id 緩存複用，方便模型在遠程主機上連續執行命令；提供 `ssh` 能力）

- `tool-musicplayer`（後台音樂播放器：`music_play` / `music_stop` / `music_status` / `music_setdir`，異步播放 MP3/WAV 文件或目錄、單曲/列表循環、隨機播放（shuffle）、音量百分比調節；`music_setdir` 持久化默認播放目錄到 `~/.dsc/musicplayer_src.txt`，`music_play` 的 path 可省略以用默認目錄，`music_status` 查詢播放模式/當前曲目/時長/音量）

- `tool-agentic-bench`（模型能力自動評分測試台：`bench_start` / `bench_next` / `bench_submit` / `bench_report`，內置 18 例運行時集成測試用例（含 CRON 機制真實調用案例 `cron_add_id` / `cron_list_roundtrip`），模型經真實工具逐一完成、插件進程自動計分並輸出彙總表與 `bench-out/report.json`；期望答案只存插件進程內、不外泄給模型，file 類用例由插件直接讀產物文件判定，可作無人值守運行——把 `DSC_WORKSPACE_ROOT` 指向臨時目錄後可用 `-input` 單回合跑全集，詳見 `plugins/tool-agentic-bench/README.md`；注意無人值守須把 stdin 重定向（如 `"" | .\dsc.exe -input …`）以關閉 `DSC_SINGLE_TURN` 單輪上限，否則模型跑 1 輪即退出、無法完成多用例循環；提供 `agentic-bench` 能力）

### Policy 插件

- `dsc-system`（核心插件混合體：**通用 dsc 類型**，單一程序承載多個駐留策略插件——各駐留插件的聲明、邏輯與文件獨立分離（每插件獨立文件，裝配只在 main.go），僅共用包名與編譯產物；經 `PluginInfo.services` 服務正交聲明（"policy"）獲宿主機械橋接工具流水線。現有駐留：fs-observation 讀前改寫策略（自 `policy-fs-observation` 遷入，對齊 DSH fs-observation-policy：str_replace/insert 前必須有本會話內先讀記錄、外部修改後 sha256 新鮮度攔截、per-session 屬主隔離）timeout 超時決策（自 `policy-timeout` 遷入，對齊 DSH timeout-policy：tool/execute 槽為 shell/subagent 裁決「活躍續命」執行域——空閒預算 DSC_SHELL_TIMEOUT / DSC_SUBAGENT_IDLE_TIMEOUT 可調、0s 禁用，宿主機械安裝看門狗與活動信號通道，無狀態）spill 外置決策（自 `policy-spill` 遷入，對齊 DSH spill-policy：tool/post-execute 槽把超閾值純文本結果全文外置為文件並 replace 為「頭尾預覽 + 定位符 + view 取回指引」，閾值 DSC_SPILL_THRESHOLD 可調、0 禁用，存儲按會話分目錄、編號跨重啓續接，盡力而為不把成功調用變失敗）skill 技能工具（自 `tool-skill` 遷入：skill / install_skill / uninstall_skill 三工具經 ToolServiceServer 疊加——TypeDsc 恆註冊工具服務，宿主 ListTools 探測非空後同時登記為 tool provider；ContextFn 注入技能索引到 system prompt，DSC_SKILLS_DIR 可調）與重複工具調用提醒（advisory 形態，見特性條目）與基礎上下文壓縮（自宿主 core/compaction.go 遷入，對齊 DSH compaction-basic：agent/pre-step 壓力驅動改寫消息列表 + agent/request-error 溢出緊急壓縮重試，LLM 摘要經 interconnect 互通、未互聯退化截斷式，per-session 狀態指紋複用，config.yaml compaction: dsc-system 選為後端——壓縮策略與狀態全歸插件，宿主零壓縮代碼）與請求面圖像預算卸載（自 agent-react-loop/image_offload.go 遷入，對齊 DSH RequestImageOffloadPolicy count 預算：agent/pre-step 槽把超預算最舊圖像引用退役為佔位文本——純瞬態投影，DSC_MAX_REQUEST_IMAGES 可調、0 不限制）；內部多策略瀑布按駐留聲明順序扇出（deny 佔槽短路、replace 結果前饋、notice 聚合）；後續核心插件逐步遷移至此）

### DSC 通用插件

- `dsc-notify`（通知音效插件：**通用 dsc 類型**，純後台程序性驅動、不暴露模型工具——經 Hook.OnEvent 訂閲宿主通用 agent 回合事件，成功（`agent/status` idle）播 success、失敗（`agent/error`）播 error，無需模型調用；內置音效 success/error/warning/info 與自定義 `.mp3/.wav`）

- `dsc-billion-context`（高級上下文管理插件：**通用 dsc 類型**，以 [acp-kernel](https://github.com/ranxianglei/acp-kernel) 算法接管 DSC 默認的 compaction-basic 壓縮機制。Go 原生重實現 acp-kernel 核心（獨立重寫非移植），經 SDK 通用事件鈎子接管：`Hook.OnEvent` 攔截 `agent/pre-step` 改寫消息列表、`Hook.ContextFn` 貢獻 ACP system prompt（對齊 DSH `ctx.systemPrompt.section`）。**模型驅動壓縮**——模型決定何時壓、壓什麼範圍，插件編排其他一切。3 層 LSM-tree：T1 標準壓縮（消息範圍→摘要）、T2 蒸餾（T1 塊→T2）、T3 凝結（T2 塊→T3）。4 個模型可見工具：`compress`（壓縮消息範圍為摘要塊、用 `bN` 塊引用觸發蒸餾）、`decompress`（恢復壓縮塊內容）、`search_context`（在壓縮塊摘要中搜索關鍵詞）、`acp_status`（查看上下文使用率與可壓縮範圍）。前綴緩存穩定：summary 原位嵌入（不堆到頭部）、不注入 `<acp>` 標籤到消息文本、nudge 作為尾部 user 消息。state 持久化：每 session 一個 JSON 文件。config.yaml `compaction: dsc-billion-context` 顯式選為後端；提供 `compaction` 能力。詳見 `plugins/dsc-billion-context/README.md`）

## 目錄結構

- `core/` — 宿主核心（插件管理、工具流水線、sandbox、subagent、workflow 工具等）

- `plugin/` — 定製版 go-plugin 庫（module path 仍 `github.com/hashicorp/go-plugin`，含宿主掛載聚合服務必需的 `GRPCClient.Broker()` 擴展）

- `plugins/` — 各插件實現（`llm-*` / `tool-*` / `agent-*` / `policy-*` / `dsc-*`）

- `sdk/` — `dsc-sdk`：聲明式插件構建器（獨立 module，插件作者只需導入 SDK）

- `workflow/` — 多 Agent Lua 編排引擎（go-lua 協程排程器執行模型編寫的腳本）

- `coderuntime/` — `run_code` 的實現：go-lua 隔離執行程序 + 按工具目錄生成 Lua SDK

- `lualib/` — go-lua 值與 Go/JSON 互轉的共享工具包（coderuntime 與 workflow 共用，避免重複實現）

- `jobs/` — 後台任務註冊表（workflow 後台運行承載）

- `session/` — 事件溯源會話（按項目路徑命名存儲，跨項目隔離）

- `proto/` — gRPC 定義與生成代碼

- `tui/` — 終端界面（Bubble Tea）

- `libs/` — vendored 本地依賴 fork（`go-lua`、`jig-lisp` 精確有理數解釋器、`sh`（mvdan.cc/sh/v3 shell 解釋器）等），經各插件 `go.mod` 的 `replace` 指入

## TUI 斜杆命令

| 命令                                                                  | 用途                                                                                                       |
| ------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `/sandbox read-only \| workspace \| full-access`                    | 切換沙箱策略（狀態欄即時顯示工作範圍）                                                                                      |
| `/approval ask \| never`                                            | 切換當前會話的審批策略（沙箱升級審批：ask 經評審通道詢問，never 自動拒絕；on/off 為 ask/never 別名）                                         |
| `/settings history <N\|off\|unlimited>`                              | 歷史注入條數（實時生效並持久化到配置）；鼠標為自動行為（模型工作期間釋放給終端原生拖選，空閒恢復應用內捕獲；永久釋放用 `DSC_DISABLE_MOUSE=1`）                                                                                        |
| `/jobs [list\|output <id>\|kill <id> [reason]]`                     | 管理後台任務（含 workflow）                                                                                       |
| `/session <id>\|new\|delete <id>` · `/sessions`                     | 多會話管理                                                                                                    |
| `/cron list\|add\|remove\|on\|off` · `/crons`                       | 定時任務                                                                                                     |
| `/plan [off]`                                                       | plan 模式開關                                                                                                |
| `/mode minimal\|standard\|creation\|ptc`                            | 切換模式（`ptc` 開啓 PTC 程序化工具組合呈現：直接把工具調用**摺疊**為唯一 `run_code`，其餘工具僅經其程序內 SDK 可調；其餘模式為 native，`run_code` 對模型隱藏） |
| `/skills` · `/help` · `/clear` · `/export`                          | 技能 / 幫助 / 清屏 / 導出                                                                                        |

## 啟動參數（CLI 旗標）

| 旗標                                       | 用途                                                                                                                                                                                                  |
| ---------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `setup`（子命令）                             | 交互式配置嚮導：基於 config.yaml 的插件狀態動態發現 LLM 提供商（並補充掃描 plugins 目錄中未聲明的 `llm-*` 插件），以行式菜單編輯基址/模型/API key、設置默認提供商，寫回 config.yaml（保留註釋），避免手動改配置文件的格式風險。僅配置 LLM 連接與 `default_llm`，為快速啓動到可用狀態；其他設置仍走 config.yaml |
| `version`（子命令）                           | 輸出版本信息：在 git 工作樹內編譯輸出「v3.0.0 短哈希」（哈希由 Go 工具鏈編譯期自動注入）；在無 `.git` 的源碼包（如 GitHub 下載 ZIP 源碼）中編譯僅輸出「v3.0.0」。版本號單一來源為項目根目錄的 VERSION 純文本文件，迭代版本號只改該文件 |
| `-mode minimal\|standard\|creation\|ptc` | 切換模式（默認 `standard`；`ptc` 開啓 PTC 程序化工具組合呈現，直接把工具調用摺疊為唯一 `run_code`；其餘模式 `run_code` 對模型隱藏）                                                                                                            |
| `-input <text>`                          | 非 TUI 自動化入口：執行一單輪後退出；stdin 為管道/文件重定向時進入多輪 stdin 驅動，直到 EOF                                                                                                                                           |
| `-headless`                              | 精簡單發模式（對齊 DSH harness headless）：僅執行 `-input` 指定的**單個任務**一次後退出，不啓動後續 stdin 多輪；任務須非空白（否則 stderr 報錯並以碼 1 退出）；**不開** ADMIN API 端口、熱重載 watcher 與 cron，專為 CI 腳本                                           |
| `-admin <addr>`                          | 管理 API 監聽地址（缺省取環境變量 `DSC_ADMIN_ADDR`，再默認迴環 `127.0.0.1:9999`；需遠程管理時用 `-admin :9999` 並配置 `DSC_ADMIN_TOKEN`）。未配置 `DSC_ADMIN_TOKEN` 不開認證                                                                |
| `-debugger`                              | 開放 `/debugger` 觀察路由（含完整會話歷史，敏感，默認不開放）                                                                                                                                                               |
| `-log [<file>]`                          | 日誌：帶文件名寫文件；僅 `-log` 時輸出到屏幕。**默認不開啓**（靜默 `io.Discard`，零噪音）。開啓即獲 DSH 等價的分級全鏈路能力（hclog 分級：`llm request` 每次 LLM 嘗試的 provider/嘗試序號/時長/finish_reason/用量、`tool executed`/`tool execution failed` 每次工具調用計時與結果、workflow 生命週期、插件裝載與會話操作）；級別經環境變量 `DSC_LOG_LEVEL` 控制（`debug\|info\|warn\|error`，默認 `info`）。插件子進程 stderr 經 go-plugin 轉發匯入同一日誌流；即使默認靜默，也可經 ADMIN `/plugins/logs` SSE 按需實時觀察 |
| `-hooks <file>`                          | 外部腳本鈎子配置（hooks.json，對齊 DSH hooks-claude-code/codex 的 Claude Code 生態複用）：在工具流水線 BeforeTool/AfterTool 階段參與裁定（veto 阻止 / 改寫參數 / 改寫結果）。**平台策略——只允許兩種形態**：`"lua"` 嚴格 LUA 腳本（本地 go-lua 進程內解釋，與 workflow 同引擎同白名單，零外部進程天然跨平台）與 `"exec"` 原生可執行文件（直接 exec 不經 shell，Windows 僅 `.exe/.com` 且複用 `CREATE_NO_WINDOW` 隱藏控制枱）；`.sh/.bat/.py` 等腳本一律拒絕（視窗下腳本引擎差異的根源），此類訴求請改寫為 LUA 鈎子。腳本契約：輸入全局（lua）/stdin JSON（exec）為 `hook_phase/tool_name/tool_args/tool_result/tool_error`，返回 `{veto,message}`（before 阻止）、`{args}`（before 改寫參數）、`{result}` 或 `{error}`（after）；lua 側另配 `json_decode/json_encode` 全局函數。單鈎子默認超時 10s（`timeout_sec` 可調），單個鈎子失敗不阻止主流程（contain，`-log` 留痕）。與插件 gRPC 鈎子（HookService，對內服務）串聯：插件先行、外部腳本隨後 |
| `-patch <file.yaml>`                     | 配置 overlay 文件（對齊 DSH `--patch`，Go 風格單 `-` 選項）：加載 YAML patch 文件合併到 config.yaml。可重複指定，按順序應用（後者覆蓋前者）。patch 文件支持兩種形式：①`- insert: [{name, type, config}]` 顯式 action（DSH 標準）；②`- name: xxx` 頂層條目（無 action，視為 insert）。同 Name 的條目替換其 Config（id-targeted override），新 Name 追加。主要用途：**啓動時自動連接 MCP 服務器**——patch 文件中聲明 `config.mcp.{server_name, endpoint}` 的條目，宿主啓動時自動連接並註冊 `mcp__<server>__<rawName>` 工具到 ToolRegistry，模型即可調用。示例：`dsc -patch config/examples/mcp-memory/mcp-memory.yaml` |

## 構建與運行

使用提供的構建器編譯主程序與所有插件：

```sh
cd builder/
go build -o builder
./builder
```

清理構建產物（開發用）：刪除主程序二進制與 plugins/ 目錄下所有插件構建產物，
但不觸碰任何源碼 / 配置 / 文檔：

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

- **啟動選版**：`LoadFromConfig` 用 `ResolveLatestBinary` 在每個插件的二進制目錄內挑選「版本號最高」的版本化文件作為初始載入（見 [core/hot\_reload\_version.go](core/hot_reload_version.go)）。

- **運行監測**：配置 `hot_reload: true` 時，宿主經 `StartHotReloadWatcher` 啟動 fsnotify + 週期掃描（見 [core/hot\_reload\_watch.go](core/hot_reload_watch.go)）。一旦某插件目錄內出現比當前運行版本更高的 `<插件名>-v<版本><擴展名>` 文件（fsnotify 即時 + ≤5s 週期兜底，≥500ms 節流防抖），即自動調用 `HotReload` 換進程，不中斷宿主與其他插件。

- **支援類型**：dsc / agent / llm / tool / policy 五類全部可熱重載。

- **原子「暫存 + 提交」**：dsc / agent / llm / tool / policy 五類插件均採用兩階段熱重載。先在不持鎖階段拉起新進程並完成全部慢速 RPC（handshake、broker 掛載、互通注入、工具列清單/策略對齊，agent 另含依賴注入），確證體康後才在極短臨界持鎖區一次性交換地圖引用並殺舊進程；預備/驗證任一環節失敗即中止並 Kill 新進程，**舊實例及其註冊原封不動**。

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

## 插件生命週期與依賴拓撲狀態機

宿主對每個插件維護一個運行狀態機（見 [core/lifecycle.go](core/lifecycle.go)），並把每次狀態遷移作為事件對外廣播（見 [core/events.go](core/events.go)），供 Admin/TUI 實時訂閲。

### 狀態值

| 狀態           | 含義                                               | 對應 DSH      |
| ------------ | ------------------------------------------------ | ----------- |
| `PENDING`    | 配置已聲明但能力依賴未滿足（如所需的 LLM/Tool 能力 provider 尚未就緒），尚不拉起子進程 | PENDING     |
| `SPAWNED`    | 子進程已創建，尚未握手                                      | （DSH 無直譯）   |
| `CONNECTING` | go-plugin/gRPC 握手、建鏈中                            | LOADING 前半段 |
| `READY`      | 業務對象已 Dispense 並註冊到 Manager，依賴/健康檢查尚未就緒          | （DSH 無直譯）   |
| `ACTIVE`     | 依賴與健康檢查就緒，可對外服務                                  | ACTIVE      |
| `UNLOADING`  | 卸載中，嘗試優雅關閉（stop hooks 先執行）                       | UNLOADING   |
| `DISPOSED`   | 已停止/已卸載（終態，不可再啟，除非重新加載）                          | DISPOSED    |
| `FAILED`     | 加載或運行失敗（終態，可被熱重載重新走一遍流程）                         | FAILED      |

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

1. **聲明與收集**：`LoadFromConfig` 收集啟用的插件條目（agent / llm / tool / policy / dsc），按 `binary_path` 解析版本化二進制（見 [core/manager.go](core/manager.go)）。依賴關係由插件二進制內的 `sdk.Config.Requires` 自描述，`config.yaml` 不含 `depends_on` 字段。
2. **Agent 先行**：agent 作為 broker 提供者**最先**拉起子進程以取得 broker（狀態 `SPAWNED → CONNECTING → READY`），但暫不激活——它依賴的 LLM/聚合 Tool 服務要等 provider 就緒後才掛載，避免 broker ConnInfo 超時窗口（宿主已把庫默認 5 秒放大為 5 分鐘，見 `PLUGIN_BROKER_CONN_TIMEOUT`）。
3. **Provider 加載與能力依賴解析**：其餘 llm/tool/policy 按順序加載，每個加載成功後解析其 `PluginInfo.Capabilities` 中的 `requires/<type>/<cap>` 編碼（`resolveRequiredDeps`），匹配已加載插件的能力鍵得到 `ResolvedDep` 列表存入運行時態 `m.resolvedDeps`；能力依賴未滿足的進入 `PENDING` 並記錄待辦。
4. **握手與校驗**：provider 加載時 `SPAWNED → CONNECTING`（建鏈）→ `READY`（元數據校驗：API 版本 `>=1.0, <2.0` + 類型一致；Tool 再經「暫存 + 提交」兩階段完成 broker 掛載、互通注入與工具列清單）。
5. **聚合服務與 Agent 激活**：provider 全部就緒後統一掛載聚合 LLM、聚合 Tool、插件通知與用户評審服務，再依 agent 的能力依賴解析得到的 primary LLM（`pickPrimaryLLM`）一次性 `RegisterServices` 注入並置 `ACTIVE`；若 agent 的 LLM 能力依賴未解析到 provider 則退回 `PENDING` 等待。
6. **運行期**：插件以 `ACTIVE` 對外服務；故障進入 `FAILED`（可被熱重載重新走流程）；熱重載採用「暫存 + 提交」兩階段，先拉起並驗證新進程，成功後才交換並卸載舊進程，預備/驗證任一環節失敗即中止並 Kill 新進程，**舊實例及其註冊原封不動**（見「Golang 插件熱更新實操」）。
7. **卸載/關機**：`Shutdown` 先停熱重載 watch 與 cron，再逐個插件 `ACTIVE → UNLOADING`（先執行對稱清理 stop hooks，如 agent 的 `Shutdown`）→ Kill 子進程 → `DISPOSED`（終態）。正常退出走此路徑；終止訊號（Ctrl+C / 直接關終端 / SIGTERM 等）同樣先 `Shutdown` 收齊插件子進程再退出，避免 defers 不執行時殘留孤兒進程（Unix 經 `signal.Notify` 捕 SIGINT/SIGTERM/SIGHUP/SIGQUIT；Windows 以 `SetConsoleCtrlHandler` 兜底點視窗關閉事件，見 [graceful\_exit.go](graceful_exit.go) / [signal\_unix.go](signal_unix.go) / [signal\_windows.go](signal_windows.go)）。
8. **動態注入與反應式重算**：運行期經 ADMIN `/plugins/load` 注入的條目若能力依賴未滿足同樣進入 `PENDING`；後續注入補足能力缺口後，`repairPendingLocked` 反應式重算所有 PENDING 插件的能力依賴（對齊 DSH/Cordis 的 `_refresh + notify`），提升等待中的 provider、並把因缺 LLM 而 `PENDING` 的 agent 重新注入 `RegisterServices` 並激活（見 [core/inject.go](core/inject.go)）。

## 許可證

本項目基於 [Apache-2.0 License](LICENSE) 許可。
