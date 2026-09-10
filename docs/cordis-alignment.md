# DSC ↔ DSH Cordis 对照关系

本文档记录 DSC 现有代码中与 DSH Cordis 框架的对应关系，作为后续开发的设计约束参考。
所有新增能力应经此对照确认是否有现成的 Cordis 等价物可复用，避免重复发明。

## 1. 统一入口

| DSH Cordis | DSC 等价 | 位置 |
|------------|---------|------|
| `ctx`（Context 对象） | `sdk.SDK` struct | `sdk/sdk.go` |

DSH 的 `ctx` 是所有能力的统一入口。DSC 的 `SDK` struct 承担等价角色——插件开发者通过 `sdk.New(config)` 创建实例，经链式方法（`sdk.Tool()` / `sdk.Hook()` / `sdk.LLM()` / `sdk.Agent()` 等）注册能力。

## 2. 事件系统

| DSH Cordis | DSC 等价 | 位置 |
|------------|---------|------|
| `EventsService` 类 | `EventBus` struct | `core/eventbus.go` |
| `ctx.on('agent/pre-step', ...)` | `Hook.OnEvent`（插件侧）+ `dispatchEventToPlugins`（宿主侧） | `sdk/hook.go` + `core/notify.go` |
| 5 种分发模式（emit/serial/bail/waterfall/parallel） | `EventBus.Emit/Serial/Bail/Waterfall/Parallel` | `core/eventbus.go` |
| 模式由事件名决定 | `eventDispatchMode` 映射表 | `core/agent_events.go` |
| 跨进程事件分发 | `dispatchEventToPlugins`（经 gRPC `PluginHookService.OnEvent`） | `core/notify.go` |

### 关键差异

DSH 是单进程：`ctx.on()` 注册的 listener 直接在同进程调用，`next()` 闭包可跨 listener 传递。

DSC 是多进程：插件在独立进程，事件经 gRPC `PluginHookService.OnEvent` 跨进程传递。`next()` 无法跨进程传递——宿主侧 `dispatchEventToPlugins` 按顺序串联每个插件，把上一个的 `result_json` 透传给下一个（实现洋葱模型）。插件侧 `Hook.OnEvent` 只返回 `(resultJSON, err)`，不感知 `next`。

## 3. 能力边界（Provide/Inject）

| DSH Cordis | DSC 等价 | 位置 |
|------------|---------|------|
| `Plugin.Base.provide('compaction')` | `Config.Provides: {"compaction": "true"}` | `sdk/sdk.go` |
| `Plugin.Base.inject(['compaction'])` | `Config.Requires: [{Type: "llm", Capability: "llm"}]` | `sdk/sdk.go` |
| Cordis registry 扫描 provide/inject 匹配 | `findProviderByCapabilityLocked` + `resolveRequiredDeps` | `core/capability_deps.go` + `core/inject.go` |
| `ctx.get('compaction')` 查询 Service | `HasPluginProvidingCapability("compaction")` | `core/capability_deps.go` |
| 同域唯一 provider（fail-loud） | `findProviderByCapabilityLocked` 多 provider 报错 | `core/capability_deps.go` |

### 关键差异

DSH 的 provide/inject 在同进程内经 Cordis fiber 匹配。DSC 的能力声明编码在 `PluginInfo.Capabilities`（provides = 普通键，requires = `requires/<type>/<cap>` 前缀键），宿主扫描匹配。LLM 类型特殊处理（允许多 provider，由 `default_llm` 显式选择）。

## 4. System Prompt 贡献

| DSH Cordis | DSC 等价 | 位置 |
|------------|---------|------|
| `ctx.systemPrompt`（Service） | `Manager.ListContext`（聚合方法） | `core/manager.go` |
| `ctx.systemPrompt.section({name, order, text})` | `Hook.ContextFn func() string` | `sdk/hook.go` |
| 33 个包使用 systemPrompt | 所有插件类型经 `PluginHookService.ListContext` 贡献 | `proto/dsc.proto` |

### 关键差异

DSH 的 `systemPrompt` 是全局 Service，section 有 `order` 字段排序。DSC 经 `PluginHookService.ListContext` RPC 聚合，按插件名稳定排序（AGENTS.md 第 6 条）。`order` 字段在 DSC 中不适用（多进程 gRPC 调用无共享 state），改为按名排序。

## 5. 工具流水线

| DSH Cordis | DSC 等价 | 位置 |
|------------|---------|------|
| `tools/pre-execute`（waterfall） | `EventToolPreExecute` + `Hook.BeforeTool` | `core/tool_pipeline.go` + `sdk/hook.go` |
| `tools/execute`（waterfall） | `EventToolExecute` | `core/tool_pipeline.go` |
| `tools/post-execute`（waterfall） | `EventToolPostExecute` + `Hook.AfterTool` | `core/tool_pipeline.go` + `sdk/hook.go` |
| `tools/result`（emit） | `EventToolResult` | `core/tool_pipeline.go` |
| `llm/request`（waterfall） | `EventLLMRequest` + `LLMRetryListener` | `core/llm_pipeline.go` |

## 6. Service 依赖注入

| DSH Cordis | DSC 等价 | 位置 |
|------------|---------|------|
| `extends Service` + `super(ctx, 'compaction')` | `CompactionEngine` interface | `core/compaction.go` |
| `declare module Context { ctx.compaction: CompactionEngine }` | `Manager.compaction` 字段 + `SetCompactionEngine`/`HasCompactionEngine` | `core/compaction.go` + `core/manager.go` |
| preset YAML 挂 compaction-basic | `config.yaml` 的 `compaction` 字段显式声明 | `core/config.go` |
| `static inject = ['llm', 'tokenMeter']` | 插件经 `Config.Requires` 声明依赖 | `sdk/sdk.go` |

### 当前状态

`CompactionEngine` 是唯一实现 Service 模式的接口。当前压缩后端接管经 `agent/pre-step` 事件机制（非 Service 调用），`SetCompactionEngine`/`HasCompactionEngine` 预留供未来 Service 注入路径。

## 7. Scope / Fiber（DSC 无等价物）

| DSH Cordis | DSC 等价 |
|------------|---------|
| `ScopedLayers` + `fiber`（上下文隔离、生命周期管理） | **无** |

DSC 多进程架构下，每个插件是独立进程，天然隔离——不需要 fiber 级 scope。agent 级别的事件过滤（DSH 的 scoped dispatch）在 DSC 中由 `WithCaller`（session ID 注入 ctx）实现。

## 8. 插件生命周期

| DSH Cordis | DSC 等价 | 位置 |
|------------|---------|------|
| preset YAML 声明插件列表 | `config.yaml` 的 `plugins` 列表 | `core/config.go` |
| Cordis fiber 加载/卸载插件 | `LoadFromConfig` / `UnloadPlugin` / `loadPluginWithBroker` | `core/manager.go` |
| `internal/service` 事件 | `Subscribe` / `publishEventLocked`（状态机推送） | `core/manager.go` |
| 热重载 | `HotReload` / `StartHotReloadWatcher` | `core/manager.go` |

## 9. 禁止的反模式（AGENTS.md 第 8 条）

以下做法在 DSC 中已被明确禁止，因为它们违反 Cordis 设计原则：

| 反模式 | 为什么禁止 | 正确做法 |
|--------|-----------|---------|
| 硬编码插件名 | 新增插件需改宿主代码 | `Provides`/`Requires` 能力边界或 `config` 显式字段 |
| env 传递运行时状态 | 子进程 spawn 时快照，动态加载/卸载不同步 | RPC（`ListContext`）或事件（`agent/pre-step`）查询 |
| 通用能力绑在特定 Service | 非 tool 类型插件无法使用 | 提升到 `PluginHookService`（所有类型注册） |
| 跨插件兼容层 | 违反 AGENTS.md 第 4 条 | 直接改签名，一步到位 |

## 设计约束总结

1. **新增通用能力时**：检查本对照表，若有 Cordis 等价物则复用，而非重新实现。
2. **新增事件时**：在 `eventDispatchMode` 映射表中声明分发模式，`Hook.OnEvent` 统一接收。
3. **新增 Service 时**：定义 interface + Manager 持有字段 + `config.yaml` 显式声明，对齐 `CompactionEngine` 模式。
4. **新增 system prompt 贡献时**：经 `Hook.ContextFn`，宿主经 `ListContext` 聚合，不直接改消息列表。
5. **跨进程通信时**：经 gRPC（`PluginHookService`）或事件（`agent/pre-step`），不经 env。
