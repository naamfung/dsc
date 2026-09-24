# DSC 插件 SDK（dsc-sdk）

獨立開發者的 Go 插件開發包：**不需要修改 DSC 主體程序或任何其他插件**，聲明式註冊
工具 / LLM / Agent / 鈎子 / 互通回調，就可產出宿主零改動即可加載的插件二進制。

## 快速開始（最小工具插件）

```go
package main

import (
        "context"
        "encoding/json"
        "dsc-sdk"
)

func main() {
        sdk := dsc.New(dsc.Config{
                Name:    "my-tool",   // 與目錄名一致：plugins/tool-my-tool/
                Version: "1.0.0",
                Type:    dsc.TypeTool,
        })

        sdk.Tool(dsc.Tool{
                Name:        "my_tool",
                Description: "Do something useful.",
                Schema:      json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`),
                Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
                        return "result", nil
                },
        })

        sdk.Serve() // 啓動 gRPC 插件服務（正常永不返回）
}
```

構建並部署：

```sh
# 獨立 module（本目錄帶 go.mod；examples/ 裏有完整可構建示例）
cd examples/tool-simple && go build -o my-tool.exe .
# 把 my-tool.exe 放進宿主 plugins/tool-my-tool/（目錄名 <type>-<name> 是宿主發現規則）
```

宿主在下次啓動（或經 ADMIN API `POST /plugins/load` 動態注入）即可加載，
**無需改動主體程序、無需任何其他插件作者配合**。

## 支持的插件類型與能力

| 類型 | 註冊 API | 能力 |
|---|---|---|
| `dsc.TypeTool` | `sdk.Tool(...)` 或 `sdk.ToolProvider(...)` | 註冊工具（可多個）或提供動態工具集；SDK 自動提供 ExecuteTool / ListTools / ListContext / 元數據 / 鈎子服務 |
| `dsc.TypeLLM` | `sdk.LLM(impl)` | 實現 `plugin.LLMProvider`（Chat / ChatStream / Name / Version / HealthCheck） |
| `dsc.TypeAgent` | `sdk.Agent(impl)` | 實現 `plugin.Agent`（Run / RunStream / RegisterServices / InjectMessage 等 11 個方法） |
| `dsc.TypePolicy` | `sdk.Policy(impl)` | 實現 `proto.FsObservationPolicyServiceServer`（宿主橋接到工具流水線） |
| `dsc.TypeDsc` | `sdk.Tool(...)` / `sdk.ToolProvider(...)` / `sdk.Hook(...)` / `sdk.Context(...)` 等可選 | 通用/純後台插件：不註冊 llm/agent/policy 服務；**可註冊工具**——宿主經 ListTools 探測並登記為 tool provider（工具集為空時跳過登記，零行為變化）；可加鈎子訂閲宿主事件廣播；目錄前綴 `dsc-` |

所有類型的元數據（Type/Name/Version/APIVersion）由 SDK 自動提供，宿主加載時校驗
`APIVersion ∈ [1.0, 2.0)`。

### 動態工具（運行時決定工具集）

工具由運行時決定（如腳本註冊、熱加載增刪）或插件為空殼（僅承載鈎子/HTTP 服務）
時，用 `sdk.ToolProvider` 提供當前工具集，宿主每次 ListTools/ExecuteTool 都會
重新求值：

```go
sdk.ToolProvider(func() []dsc.Tool {
        // 返回當前工具列表；空集合法（空殼工具插件）
        return []dsc.Tool{{
                Name: "dyn", Description: "dynamic", Schema: json.RawMessage(`{}`),
                Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
                        return "ok", nil
                },
        }}
})
```

### 工具能力聲明（capability）

`dsc.Tool.Capabilities []string` 可選字段聲明能力標籤（wire token 見 `proto.Tool.capabilities`）。
聲明瞭 `dsc.CapabilityRequiresNeverApproval`（值 `"requires-never-approval"`）的工具要求宿主
會話審批策略為 `never` 才允許調用：否則宿主在執行前拒絕並提示先設 `approval=never`
（`DSC_APPROVAL=never` 或 TUI `/approval never`），避免無人值守評測在 ask 下逐工具彈窗授權。
新增同類需要前置審批門控的工具，只需在工具聲明裏加該能力標籤，宿主無需改動（按能力而非插件名識別）。

### 聲明式依賴（Config.Requires）——對齊 DSH/Cordis 的 provide + inject 模型

DSC 的能力依賴模型對齊 DSH/Cordis 的 `provide` + `inject` 雙向聲明機制：

- **`Config.Provides map[string]string`** ——聲明本插件**提供**哪些能力（對齊 Cordis `provide`）。
  鍵為能力名（如 `"filesystem"`、`"skill"`、`"cron"`），值為能力屬性字符串（`"true"` 表示啓用）。
  其他插件經 `Requires` 聲明對此能力的依賴時，宿主掃描 `PluginInfo.Capabilities` 找到首個
  聲明該能力的插件並建立依賴關係。
  - LLM 插件自動提供 `CapabilityLLM="llm"` 能力（由 `llmSdkMetadataServer` 硬編碼），無需重複聲明。
  - tool/agent/policy/dsc 類型插件若需聲明自定義能力，在 `Provides` 中填入即可。

- **`Config.Requires []CapabilityRequirement`** ——聲明本插件**依賴**哪些能力（對齊 Cordis `inject`）。
  宿主據此在加載時掃描已加載插件的能力鍵自動匹配依賴來源，**不落盤**——能力依賴由插件二進制
  內的 `sdk.Config.Requires` 自描述，`config.yaml` 不含 `depends_on` 字段。

```go
// tool-filesystem 聲明提供 "filesystem" 能力
sdk := dsc.New(dsc.Config{
    Name:    "filesystem",
    Type:    dsc.TypeTool,
    Version: "1.0.0",
    Provides: map[string]string{
        "filesystem": "true",
    },
})

// 另一個插件聲明依賴 "filesystem" 能力
sdk := dsc.New(dsc.Config{
    Name:    "my-tool",
    Type:    dsc.TypeTool,
    Version: "1.0.0",
    Requires: []dsc.CapabilityRequirement{
        // 依賴一個提供 "filesystem" 能力的 tool 插件（即上面的 tool-filesystem）
        {Type: "tool", Capability: "filesystem"},
        // 依賴一個提供 "llm" 能力的 LLM 插件（所有 LLM 插件自動提供）
        {Type: "llm", Capability: "llm"},
    },
})
```

**機制約定**：
- 插件在 `PluginInfo.Capabilities` 中以兩種鍵編碼能力信息：
  - 普通能力鍵（如 `"filesystem": "true"`）——表示本插件**提供**了該能力（來自 `Config.Provides`）；
  - `requires/<type>/<capability>` 前綴鍵（如 `"requires/llm/llm": "true"`）——表示本插件**依賴**
    某類型插件的某能力（來自 `Config.Requires`）。
- 宿主在 `install_dsc_plugin` / `load_dsc_plugin` / `injectionEntryLocked` / `LoadFromConfig`
  調用流程中讀取新加載插件的 `PluginInfo`，調用 `resolveRequiredDeps`：
  - 掃描已加載的 `Type` 類型插件的 capabilities（普通能力鍵），找到首個聲明該能力且
    值非 `"false"` 的插件（按名升序，穩定輸出）；
  - 把匹配結果（`ResolvedDep` 列表）存入運行時態 `m.resolvedDeps`；
  - 避免自引用：插件自身聲明的 capability 不被解析為自身的依賴來源；
  - `persist=true` 時把插件條目本身寫回 `config.yaml`（**不含 `depends_on` 字段**）。
- 反應式重算：provider 加載/卸載時，`repairPendingLocked` 重算所有 PENDING 插件的能力
  依賴（對齊 DSH/Cordis 的 `_refresh + notify`），自動提升依賴已就緒的 PENDING 插件。
- agent 的 primary LLM 選擇：從 `m.resolvedDeps[agentName]` 中取首個 `Type=="llm"` 且
  `ProviderName` 非空的項，以其 `ProviderName` 作為 primary LLM（多條 llm 依賴時按
  Capability 名升序取首個，穩定選擇）。

**`SetInterconnect` ≠ `Requires`（重要區分）**：
`SetInterconnect`（`ic.LLM()` / `ic.Tool()` / `ic.Agent()`）是**運行時可選**訪問宿主聚合服務的
模式——插件容忍 `nil` 並優雅降級。這與 `Config.Requires`（加載時硬依賴）是**不同**的概念：
`Requires` 會在依賴未滿足時阻止插件激活（PENDING），而 `SetInterconnect` 僅在調用時檢查
服務是否可用。不要把運行時可選訪問誤寫成 `Requires`，否則會破壞最小配置場景。

## 鈎子（參與宿主流水線，無需任何插件配合）

```go
sdk.Hook(dsc.Hook{
        // 工具執行前：返回改寫後的參數 JSON；err 非 nil 表示否決（阻止執行）
        BeforeTool: func(ctx context.Context, toolName, argumentsJSON string) (string, error) {
                return argumentsJSON, nil
        },
        // 工具執行後：按本次調用的原始參數改寫結果/錯誤
        AfterTool: func(ctx context.Context, toolName, argumentsJSON, result, toolErr string) (string, string) {
                return result, toolErr
        },
        // 宿主事件訂閲（異步廣播：turn/start、tool/result 等）
        OnEvent: func(ctx context.Context, eventType, dataJSON string) {},
})
```

宿主在工具流水線 **pre-execute（沙箱策略 → BeforeTool）→ execute → post-execute
（AfterTool）** 中按插件加載順序調用鈎子，任一插件 veto 即阻止執行。

## 互通（插件間互不感知地調用宿主能力）

```go
sdk.SetInterconnect(func(ctx context.Context, ic *dsc.Interconnect) error {
        // ic.LLM()      —— 宿主聚合 LLM（含多 provider 路由，Thinking/工具調用）
        // ic.Tool()     —— 宿主聚合 Tool（經宿主流水線調用任意工具插件）
        // ic.Notifier() —— 宿主插件通知客户端（把實例傳給第三方場景用）
        // ic.Notify(name, dataJSON) —— 發佈事件到宿主總線（TUI 喚醒/其他插件訂閲）
        // 在此緩存 ic 供工具 Handler 使用
        return nil
})
```

宿主掛載聚合服務後回調一次；獨立插件之間互不感知——經宿主聚合路由調用其他
插件能力，無需知道對方的存在。

## Agent 類型插件接入宿主服務（AgentBroker）

`dsc.TypeAgent` 插件在 gRPC server 建立時收到 SDK 對 go-plugin broker 的隔離封裝
`*dsc.AgentBroker`——**插件代碼無需 import go-plugin**：

```go
var ab *dsc.AgentBroker
sdk.AgentBroker(func(b *dsc.AgentBroker) error {
        ab = b // 緩存；宿主經 RegisterServices/SetUserQuestionsService 下發服務 ID 後使用
        return nil
})
```

宿主隨後調用 `agent.RegisterServices(ctx, llmID, toolID)`（LLM 聚合服務 / Tool 聚合
服務）與 `SetUserQuestionsService(uqID)`，agent 在回調裏用 `AgentBroker` 建立連接：

```go
func (a *MyAgent) RegisterServices(ctx context.Context, llmID, toolID uint32) error {
        llm, err := ab.DialLLM(llmID)          // 宿主聚合 LLM 客户端
        tool, err := ab.DialTool(toolID)       // 宿主聚合 Tool 客户端
        // 需要自建 proto client 時：conn, err := ab.Dial(id)
        return nil
}
```

便捷方法：`DialLLM / DialTool / DialNotify / DialUserQuestions`（返回封裝客户端，
`serviceID == 0` 或未注入時返回 `nil`）與通用 `Dial`（返回 `*grpc.ClientConn`）。

## 進程上下文

```go
env := dsc.ReadEnv() // Mode / WorkspaceRoot / SessionDir / ContextWindow / ...
```

宿主統一注入 `DSC_*` 環境變量（workspace 根、模式、會話目錄、上下文容量等），
插件只讀即可，無需關心注入細節。

## 統一文件 IO

所有插件讀寫文件統一經 SDK 助手，獲得一致的錯誤包裝（`read <path>: ...` /
`write <path>: ...`）與防禦性 recover（panic 轉錯誤返回，絕不 crash 插件進程
導致 LLM 連接中斷）。錯誤信息裏的路徑統一**正斜杆**呈現（`filepath.ToSlash`），
與內部 POSIX shell（tool-filesystem 的 mvdan/sh）風格一致——Windows 上底層
`os.*` 錯誤內嵌反斜杆路徑，直接回顯會給模型造成兩種風格混雜的混亂；底層錯誤
仍經 `Unwrap` 保留，`errors.Is/As` 照常可用。

```go
data, err := dsc.ReadFile(path)                    // 讀取
err  = dsc.WriteFile(path, data)                   // 寫入（默認 0644）
err  = dsc.MkdirAll(dir)                           // 建目錄（默認 0755）
root := dsc.WorkspaceRoot()                        // 工作空間根（DSC_WORKSPACE_ROOT，回退 cwd）
abs, err := dsc.AbsPath(path)                      // 絕對路徑規範化
```

注意：本層不做 workspace 越界檢查——沙箱策略由宿主工具流水線統一判定，插件
自行判定反而會在 full-access 模式下誤拒 workspace 外路徑。

## 後台 goroutine

所有後台 goroutine 一律經 `SafeGoroutine` 啓動（禁止裸 `go func()`）：

```go
dsc.SafeGoroutine(func() {
	// 後台任務；panic 被 recover 並連同調用棧打到 stderr，絕不 crash 插件進程
})
```

goroutine 裏的 panic 無法被工具執行鏈路的 recover 捕獲（recover 只對當前
goroutine 有效），不在此接住會直接幹掉整個插件進程導致 LLM 連接中斷。

## LUA 插件開發

SDK 面向 Go 插件；**LUA 工具開發走既有的 tool-lua-host 通路**（無需本 SDK）：

- 腳本目錄：`plugins/tool-lua-host/scripts/<name>/main.lua`；
- LUA API：`dsc.register_tool` / `dsc.llm.chat` / `dsc.tool.call` / `dsc.notify.emit`
  / `dsc.store.*` / `dsc.hook.before_tool|after_tool|on_event` / `dsc.job.*`；
- 腳本在 `creation` 模式下可熱加載；非創造模式僅加載啓動時已存在的腳本；
- 參考樣例：`plugins/tool-lua-host/scripts/example/main.lua`。

LUA 適合模型自行開發功能與輕量工具；需要複雜邏輯/依賴/性能時用本 SDK 寫 Go 插件。

## 模塊結構

```
sdk/
  sdk.go           入口：New / Tool / LLM / Agent / Hook / SetInterconnect / OnStart / OnStop / Serve
  tool.go          工具服務端（ExecuteTool / ListTools / ListContext / SetInterconnect）
  hook.go          鈎子服務端（BeforeTool / AfterTool / OnEvent 的語義化封裝）
  metadata.go      元數據服務
  interconnect.go  宿主能力客户端集（LLM / Tool / Notify）
  env.go           進程上下文（DSC_* 環境變量）
  fs.go            統一文件 IO 助手（ReadFile / WriteFile / MkdirAll / WorkspaceRoot / AbsPath）
  safe.go          後台 goroutine 兜底（SafeGoroutine：recover + stderr 記錄）
  examples/        可構建示例：tool-simple / hook-tool / llm-proxy
```

## 依賴説明

`sdk/` 是獨立 Go module（`dsc-sdk`）。**API 層已隔離 go-plugin**：公開接口（Tool /
ToolProvider / Hook / SetInterconnect / AgentBroker）均不含 go-plugin 類型，插件代碼
無需 `import "github.com/hashicorp/go-plugin"`——一律以 SDK 的封裝（AgentBroker 等）
接入宿主能力。

go.mod 中統一以**本倉庫定製版 go-plugin 為準**（不要使用官方版）：`dsc` 的宿主
core 包依賴定製版 `GRPCClient.Broker()` 擴展（宿主掛載聚合服務所必需），官方版
無等價 API，用官方版會導致編譯失敗。模板：

```go
require (
        dsc v0.0.0
        dsc-sdk v0.0.0
)

replace dsc => <dsc 倉庫路徑>           // 宿主契約（core/proto）
replace dsc-sdk => <dsc 倉庫路徑>/sdk   // 本 SDK
replace github.com/hashicorp/go-plugin => <dsc 倉庫路徑>/plugin // 定製版（含 Broker 擴展）
```

獨立開發者在自己的倉庫裏聲明上述 replace 即可（`examples/*` 的 go.mod 是完整模板，
可複製改路徑）。未來將 dsc / 定製版 go-plugin 發佈為遠程 module 時，go-plugin 的
replace 指向該遠程地址即可——**始終以我們的定製版為準，不引入官方版**。
