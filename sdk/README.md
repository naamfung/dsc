# DSC 插件 SDK（dsc-sdk）

独立开发者的 Go 插件开发包：**不需要修改 DSC 主体程序或任何其他插件**，声明式注册
工具 / LLM / Agent / 钩子 / 互通回调，就可产出宿主零改动即可加载的插件二进制。

## 快速开始（最小工具插件）

```go
package main

import (
	"context"
	"encoding/json"
	"dsc-sdk"
)

func main() {
	sdk := dsc.New(dsc.Config{
		Name:    "my-tool",   // 与目录名一致：plugins/tool-my-tool/
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

	sdk.Serve() // 启动 gRPC 插件服务（正常永不返回）
}
```

构建并部署：

```sh
# 独立 module（本目录带 go.mod；examples/ 里有完整可构建示例）
cd examples/tool-simple && go build -o my-tool.exe .
# 把 my-tool.exe 放进宿主 plugins/tool-my-tool/（目录名 <type>-<name> 是宿主发现规则）
```

宿主在下次启动（或经 ADMIN API `POST /plugins/load` 动态注入）即可加载，
**无需改动主体程序、无需任何其他插件作者配合**。

## 支持的插件类型与能力

| 类型 | 注册 API | 能力 |
|---|---|---|
| `dsc.TypeTool` | `sdk.Tool(...)` 或 `sdk.ToolProvider(...)` | 注册工具（可多个）或提供动态工具集；SDK 自动提供 ExecuteTool / ListTools / ListContext / 元数据 / 钩子服务 |
| `dsc.TypeLLM` | `sdk.LLM(impl)` | 实现 `plugin.LLMProvider`（Chat / ChatStream / Name / Version / HealthCheck） |
| `dsc.TypeAgent` | `sdk.Agent(impl)` | 实现 `plugin.Agent`（Run / RunStream / RegisterServices / InjectMessage 等 11 个方法） |
| `dsc.TypePolicy` | `sdk.Policy(impl)` | 实现 `proto.FsObservationPolicyServiceServer`（宿主桥接到工具流水线） |

所有类型的元数据（Type/Name/Version/APIVersion）由 SDK 自动提供，宿主加载时校验
`APIVersion ∈ [1.0, 2.0)`。

### 动态工具（运行时决定工具集）

工具由运行时决定（如脚本注册、热加载增删）或插件为空壳（仅承载钩子/HTTP 服务）
时，用 `sdk.ToolProvider` 提供当前工具集，宿主每次 ListTools/ExecuteTool 都会
重新求值：

```go
sdk.ToolProvider(func() []dsc.Tool {
	// 返回当前工具列表；空集合法（空壳工具插件）
	return []dsc.Tool{{
		Name: "dyn", Description: "dynamic", Schema: json.RawMessage(`{}`),
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "ok", nil
		},
	}}
})
```

## 钩子（参与宿主流水线，无需任何插件配合）

```go
sdk.Hook(dsc.Hook{
	// 工具执行前：返回改写后的参数 JSON；err 非 nil 表示否决（阻止执行）
	BeforeTool: func(ctx context.Context, toolName, argumentsJSON string) (string, error) {
		return argumentsJSON, nil
	},
	// 工具执行后：按本次调用的原始参数改写结果/错误
	AfterTool: func(ctx context.Context, toolName, argumentsJSON, result, toolErr string) (string, string) {
		return result, toolErr
	},
	// 宿主事件订阅（异步广播：turn/start、tool/result 等）
	OnEvent: func(ctx context.Context, eventType, dataJSON string) {},
})
```

宿主在工具流水线 **pre-execute（沙箱策略 → BeforeTool）→ execute → post-execute
（AfterTool）** 中按插件加载顺序调用钩子，任一插件 veto 即阻止执行。

## 互通（插件间互不感知地调用宿主能力）

```go
sdk.SetInterconnect(func(ctx context.Context, ic *dsc.Interconnect) error {
	// ic.LLM()      —— 宿主聚合 LLM（含多 provider 路由，Thinking/工具调用）
	// ic.Tool()     —— 宿主聚合 Tool（经宿主流水线调用任意工具插件）
	// ic.Notifier() —— 宿主插件通知客户端（把实例传给第三方场景用）
	// ic.Notify(name, dataJSON) —— 发布事件到宿主总线（TUI 唤醒/其他插件订阅）
	// 在此缓存 ic 供工具 Handler 使用
	return nil
})
```

宿主挂载聚合服务后回调一次；独立插件之间互不感知——经宿主聚合路由调用其他
插件能力，无需知道对方的存在。

## Agent 类型插件接入宿主服务（AgentBroker）

`dsc.TypeAgent` 插件在 gRPC server 建立时收到 SDK 对 go-plugin broker 的隔离封装
`*dsc.AgentBroker`——**插件代码无需 import go-plugin**：

```go
var ab *dsc.AgentBroker
sdk.AgentBroker(func(b *dsc.AgentBroker) error {
	ab = b // 缓存；宿主经 RegisterServices/SetUserQuestionsService 下发服务 ID 后使用
	return nil
})
```

宿主随后调用 `agent.RegisterServices(ctx, llmID, toolID)`（LLM 聚合服务 / Tool 聚合
服务）与 `SetUserQuestionsService(uqID)`，agent 在回调里用 `AgentBroker` 建立连接：

```go
func (a *MyAgent) RegisterServices(ctx context.Context, llmID, toolID uint32) error {
	llm, err := ab.DialLLM(llmID)          // 宿主聚合 LLM 客户端
	tool, err := ab.DialTool(toolID)       // 宿主聚合 Tool 客户端
	// 需要自建 proto client 时：conn, err := ab.Dial(id)
	return nil
}
```

便捷方法：`DialLLM / DialTool / DialNotify / DialUserQuestions`（返回封装客户端，
`serviceID == 0` 或未注入时返回 `nil`）与通用 `Dial`（返回 `*grpc.ClientConn`）。

## 进程上下文

```go
env := dsc.ReadEnv() // Mode / WorkspaceRoot / SessionDir / ContextWindow / ...
```

宿主统一注入 `DSC_*` 环境变量（workspace 根、模式、会话目录、上下文容量等），
插件只读即可，无需关心注入细节。

## LUA 插件开发

SDK 面向 Go 插件；**LUA 工具开发走既有的 tool-lua-host 通路**（无需本 SDK）：

- 脚本目录：`plugins/tool-lua-host/scripts/<name>/main.lua`；
- LUA API：`dsc.register_tool` / `dsc.llm.chat` / `dsc.tool.call` / `dsc.notify.emit`
  / `dsc.store.*` / `dsc.hook.before_tool|after_tool|on_event` / `dsc.job.*`；
- 脚本在 `creation` 模式下可热加载；非创造模式仅加载启动时已存在的脚本；
- 参考样例：`plugins/tool-lua-host/scripts/example/main.lua`。

LUA 适合模型自行开发功能与轻量工具；需要复杂逻辑/依赖/性能时用本 SDK 写 Go 插件。

## 模块结构

```
sdk/
  sdk.go           入口：New / Tool / LLM / Agent / Hook / SetInterconnect / OnStart / OnStop / Serve
  tool.go          工具服务端（ExecuteTool / ListTools / ListContext / SetInterconnect）
  hook.go          钩子服务端（BeforeTool / AfterTool / OnEvent 的语义化封装）
  metadata.go      元数据服务
  interconnect.go  宿主能力客户端集（LLM / Tool / Notify）
  env.go           进程上下文（DSC_* 环境变量）
  examples/        可构建示例：tool-simple / hook-tool / llm-proxy
```

## 依赖说明

`sdk/` 是独立 Go module（`dsc-sdk`）。**API 层已隔离 go-plugin**：公开接口（Tool /
ToolProvider / Hook / SetInterconnect / AgentBroker）均不含 go-plugin 类型，插件代码
无需 `import "github.com/hashicorp/go-plugin"`——一律以 SDK 的封装（AgentBroker 等）
接入宿主能力。

go.mod 中统一以**本仓库定制版 go-plugin 为准**（不要使用官方版）：`dsc` 的宿主
core 包依赖定制版 `GRPCClient.Broker()` 扩展（宿主挂载聚合服务所必需），官方版
无等价 API，用官方版会导致编译失败。模板：

```go
require (
	dsc v0.0.0
	dsc-sdk v0.0.0
)

replace dsc => <dsc 仓库路径>           // 宿主契约（core/proto）
replace dsc-sdk => <dsc 仓库路径>/sdk   // 本 SDK
replace github.com/hashicorp/go-plugin => <dsc 仓库路径>/plugin // 定制版（含 Broker 扩展）
```

独立开发者在自己的仓库里声明上述 replace 即可（`examples/*` 的 go.mod 是完整模板，
可复制改路径）。未来将 dsc / 定制版 go-plugin 发布为远程 module 时，go-plugin 的
replace 指向该远程地址即可——**始终以我们的定制版为准，不引入官方版**。
