# DSC 改进说明

本补丁基于 https://github.com/naamfung/dsc master 分支（commit e2bd4d2）增加了两项功能，
并使用 Go 1.27.1（最新版）验证 build + test **全过**（含修复两处预先存在的跨平台测试 bug
+ 一处时间戳冲突；以及对齐 DSH/Cordis 全面清理旧有按名依赖代码）。

## 验证环境

- Go 版本：go1.27.1 linux/amd64（最新稳定版，2026 年发布）
- 验证命令：
  ```
  go build ./...                 # dsc 主模块
  (cd sdk && go build ./...)     # dsc-sdk 模块
  (cd plugins/tool-agentic-bench && go build ./...)  # bench 插件
  go test . ./core/ -count=1     # 宿主主程序 + 核心（全过）
  (cd sdk && go test ./...)      # SDK 全过
  (cd plugins/tool-agentic-bench && go test ./...)  # bench 插件全过
  gofmt -l .                     # 无未格式化文件
  ```

## 改动 1：插件机制改进 — 全面采用能力依赖模型（对齐 DSH/Cordis，删除按名依赖旧代码）

### 设计动机

DSC 原有的 `DependsOn`（按插件名指定依赖，如 `depends_on.llm: "llm-openai"`）属旧有落后
机制——克隆 DSH（`github.com/deepseek-ai/deepseek-harness`）到同级目录调研后，确认 DSH/
Cordis 用的是纯能力边界的依赖定义方式：
- 插件经 `provide` 字段显式声明提供哪些能力（service name）；
- 插件经 `inject` 字段声明依赖哪些能力；
- **不按插件名**指定依赖——`inject: ['llm', 'terminals', 'systemPrompt']` 而非
  `depends_on.llm: "llm-openai"`；
- Cordis 的 Fiber 在 provider 加载/卸载时**反应式重算**所有依赖者的状态（`_refresh` +
  `notify`），自动 reload 或 unload。

依据 `AGENTS.md` 第4条「禁止保留 Deprecated 代码」——本项目仍在高速迭代，保留 Deprecated
代码只会造成代码膨胀、堆积无用代码并让「旧实现是否仍在使用」的判断混乱——本次彻底删除
按名依赖机制，全面采用能力依赖模型。

### 实现

- **删除的旧代码**：
  - `core/config.go`：`PluginEntry.DependsOn` 字段、`PluginDepends` 类型；
  - `core/manager.go`：`CheckCircularDependencies`、`declaredPluginDeps`、`topoSortPlugins`
    三个函数（按名依赖的拓扑排序）；
  - `config/config.yaml` / `config/config.example.yaml`：`depends_on` 字段；
  - `main.go` / `setup.go`：读 `agent.depends_on.llm` 选 primary LLM 的代码。

- **新增的能力依赖模型**：
  - SDK：`dsc.Config.Requires []CapabilityRequirement`（已存在，对齐 DSH/Cordis 的 `inject`）；
  - SDK：`PluginInfo.Capabilities` 普通能力键（如 `supports_images: "true"`、`cron: "true"`）
    表示本插件**提供**了该能力（对齐 DSH/Cordis 的 `provide`）；
  - Host：`ResolvedDep` 类型（`{Type, Capability, ProviderName}`）记录已解析的能力依赖；
  - Host：`m.resolvedDeps map[string][]ResolvedDep` 运行时态存储所有插件的能力依赖解析结果；
  - Host：`resolveRequiredDeps(info, selfName)` 扫描已加载插件的能力键，匹配首个声明该
    能力的插件并记入 `ProviderName`；尚未找到 provider 的项 `ProviderName` 为空串；
  - Host：`depsSatisfiedLocked` / `agentDepsSatisfiedLocked` / `providerDepsSatisfiedLocked`
    判定能力依赖是否全部已解析到 provider 且对应 provider 已加载；
  - Host：`pickPrimaryLLM` 从 `ResolvedDep` 列表中选首个 `Type=="llm"` 且 `ProviderName` 非空
    的项作为 agent 的 primary LLM（多条 llm 依赖时按 Capability 名升序取首个，稳定选择）。

- **反应式重算**（对齐 DSH/Cordis 的 `_refresh + notify`）：
  - `LoadFromConfig` 启动期：每个 provider 加载成功后调用 `autoResolveAndPersistDepsLocked`
    解析其能力依赖填入 `m.resolvedDeps`；
  - `repairPendingLocked` 动态注入期：每轮扫描 PENDING 插件前先调用 `resolveRequiredDeps`
    重算其 `Requires` 是否被新加载的插件所提供——更新 `m.resolvedDeps` 后再判定依赖是否
    就绪；新加载的 provider 可能补足先前 PENDING 插件的能力依赖，触发其提升（与 DSH/Cordis
    中 provider 加载触发依赖者 `_refresh` 的反应式语义一致）；
  - `reactivateAgentLocked` 从 `m.resolvedDeps[agentName]` 读 primary LLM 而非旧的
    `agentEntries[name].DependsOn.LLM`。

- **持久化解耦**（两条契约独立）：
  1. 自动解析的时机无限制——只要插件 `PluginInfo` 可读即可，不要求依赖已满足；
  2. 解析得到的 `ResolvedDep` 列表一律写入运行时态 `m.resolvedDeps`，供运行时态查询与反应式
     重算使用，**无论 persist 开关如何**；
  3. `persist=true` 时把 entry 写回 `config.yaml`（**不含 `depends_on` 字段**——能力依赖
     由插件二进制内的 `sdk.Config.Requires` 自描述，无需落盘）；
     `persist=false`（孤儿插件经 `load_dsc_plugin` 默认载入）时**绝不写配置**。

- **附带修复预先存在的 `backupConfig` 时间戳冲突**：原实现用 `config.yaml.<毫秒>.bak` 命名，
  同毫秒内多次备份产生同名文件互相覆盖（曾致 `TestLoadUnloadDscPluginE2E` 偶发失败——原始
  master 上 5 次连测全失败）。改为 `<毫秒>.<进程内自增序号>.bak`，新增 `Manager.backupSeq
  atomic.Int64` 计数器，确保唯一性。修复后 10 次连测全过。

### 测试
- `core/capability_deps_test.go`：覆盖解析、解码、自动解析 LLM/Tool 依赖、保留显式声明、
  无 provider、自引用排除、多能力解析、`capability=false` 不匹配、`depsSatisfiedLocked`、
  `pickPrimaryLLM`、persist=false 不写配置 + 运行时态可查询、persist=true 写插件条目但不
  含 depends_on 等场景（共 13 例）。
- `core/inject_test.go`：`TestAgentDepsSatisfiedByCapability`、`TestAgentReactivate`、
  `TestReactivateNoopWhenNotPending`、`TestRepairPendingReactivateLoadedAgent` 全部改为基于
  `m.resolvedDeps` 验证能力依赖解析与反应式激活。
- 删除 `core/topo_test.go`（测试已删除的 `topoSortPlugins` 函数，按 AGENTS.md 第4条删除）。

## 改动 2：在 BENCH 评测插件中增加 CRON 案例的测试题目

### 设计动机
要让模型在评测中真实调用 DSC 的 CRON 机制——而不仅经 TUI 斜杆命令或管理 API 调度。
为此宿主侧需要先把 CRON 操作以「模型工具」形式暴露给 agent，再设计 bench 案例用例。

### 实现
- **宿主侧新增 CRON 工具族**（`core/cron_tools.go`）：
  - `cron_add` / `cron_list` / `cron_remove` / `cron_set_enabled` 四个工具，把宿主侧的
    cron 调度器（`core/cron.go`、`cron/scheduler.go`）以工具形式暴露给 agent。
  - 变更类工具（`cron_add` / `cron_remove` / `cron_set_enabled`）声明 `ApprovalRequester`，
    在审批策略非 `never` 时被前置门控拦截（对齐 `install_dsc_plugin` 等高影响变更）；
    `cron_list` 是只读工具，无需审批。
  - 在 `core/manager.go` 中注册到 `toolRegistry`，模型可直接调用。
  - 严格命名校验（`[A-Za-z0-9_-]`）、cron 表达式校验、空值检查。

- **BENCH 新增 CRON 案例用例**（`plugins/tool-agentic-bench/cases.go`）：
  - `cron_add_id`（file/regex，NoLeak=true，weight 1）：要求模型用 `cron_add` 真实添加
    一个定时任务，把宿主返回的 `cron-<digits>` id 落盘到 reply.txt。**模型无法凭空捏造
    合法 id**——只有真实调用 `cron_add` 才能拿到。
  - `cron_list_roundtrip`（file/regex，NoLeak=true，weight 2）：多步骤全链路——
    `cron_add → cron_list → cron_set_enabled(false) → cron_list 验证 disabled → 落盘 id`，
    验证模型能完成 CRON 机制的完整往返。
  - 两个用例都标记 `NoLeak=true`（期望格式即任务规格，无可保密的答案）。

### 测试
- `core/cron_tools_test.go`：覆盖工具注册、schema 合法性、坏输入拒绝、调度器未启动时
  的行为、审批声明、端到端 add → list → set_enabled → remove 流程、非法 cron 表达式拒绝。
- `plugins/tool-agentic-bench/main_test.go`：更新 TestBuildReportRoundTripPreservesFields
  适配 18 用例（原 16），新增 TestCronCasesDeclared、TestCronCasesRegexAcceptsValidID、
  TestNoAnswerLeakCronCases 验证新用例声明正确、regex 接受合法 id 并拒绝非法输入。
- 全部测试通过。

## 改动 3：修复两处预先存在的跨平台测试 bug

### 3.1 `TestSandboxWorkspaceAllowsRealPathCaseInsensitive`（core/sandbox_test.go）

**症状**：在 Linux ext4（大小写敏感 FS）上失败。

**根因**：跳过条件 `alt == root+"/inside.txt"` 是字符串相等比较——只在「`strings.ToLower(root)`
没有改变 root」时才跳过。但在 Linux 上 `t.TempDir()` 返回的路径如
`/tmp/TestSandbox492079869/ws`（含大写字母 T、S），`strings.ToLower` 会把它变成
`/tmp/testsandbox492079869/ws/inside.txt`，与原路径不等，于是测试**不跳过继续跑**——
但 Linux 大小写敏感，小写路径下的目录根本不存在，`CanonicalPath` 解析失败、`inWorkspace`
返回 false、沙箱拒绝，测试假阴性失败。

**修复**：新增 `isFileSystemCaseInsensitive(path)` 探测函数——在 path 同目录下创建一个含
大写字母的临时子目录（`CaseProbe-NNN`），再以全小写形式 `os.Stat` 它：能 stat 到 → FS
大小写不敏感（Windows/macOS 默认 FS）；找不到 → 大小写敏感（Linux ext4 等）。
跳过条件改用此函数判定，是 FS 的真实属性而非路径字符串是否含大写字母。

### 3.2 `TestDefaultSessionIDFollowsWorkspaceRoot`（core/event_test.go）

**症状**：在 Linux 上失败。

**根因**：测试断言 `C:\Users\Administrator\Desktop\DeepClean` 经 `SessionKeyForProject`
转换后应得 `C--Users-Administrator-Desktop-DeepClean`——该转换依赖 `filepath.ToSlash`
把 Windows 反斜杠 `\` 转成正斜杠 `/`。但 `filepath.ToSlash` 在 Linux 上是 no-op：
Linux 上 `\` 是合法文件名字符（不是分隔符），故 `\` 不被转换，原样保留在结果里。
测试硬编码了 Windows 路径样例，没考虑平台差异。

**修复**：测试改为按 `runtime.GOOS` 选平台典型样例——Windows 上仍断言
`C:\Users\...` → `C--Users-...`；Linux/macOS 上用 `/home/jor/DeepClean` → `home-jor-DeepClean`。
同时新增一条跨平台恒成立的断言：`DefaultSessionID() == SessionKeyForProject(WorkspaceRoot)`。

## 文件清单

### 删除
- `core/topo_test.go`：测试已删除的 `topoSortPlugins` 函数。

### 修改
- `README.md`：声明式依赖条目改为「对齐 DSH/Cordis 的 provide + inject 模型」；启动流程
  改为按能力依赖解析；状态表 PENDING 含义更新。
- `config/config.yaml` / `config/config.example.yaml`：移除 `depends_on` 字段。
- `config_public_test.go`：移除对 `agent.DependsOn.Tools` 的检查（字段已删除）。
- `core/config.go`：删除 `PluginEntry.DependsOn` 字段与 `PluginDepends` 类型。
- `core/config_persist.go`：注释更新（不再有 depends_on 字段）。
- `core/cron_tools.go`：CRON 工具族（cron_add/list/remove/set_enabled）。
- `core/cron_tools_test.go`：CRON 工具测试。
- `core/dsc_plugin_tool.go`：backupConfig 文件名加进程内自增序号，修复同毫秒冲突。
- `core/dsc_plugin_tool_test.go`：TestBackupConfig 正则适配新文件名格式。
- `core/event_test.go`：TestDefaultSessionIDFollowsWorkspaceRoot 修复跨平台假设。
- `core/grpc_llm.go`：导出 `NewLLMServiceServer` 构造函数。
- `core/inject.go`：重写——`injectionEntryLocked` 集成能力解析；新增
  `autoResolveAndPersistDepsLocked`（两条契约独立：解析结果总存运行时态 m.resolvedDeps；
  持久化只 persist=true 才做）；新增 `GetResolvedDeps` 运行时态查询入口；
  `repairPendingLocked` 加入反应式能力重算（对齐 DSH/Cordis _refresh + notify）；
  `reactivateAgentLocked` 改为从 `m.resolvedDeps` 读 primary LLM（`pickPrimaryLLM`）；
  删除 `knownPluginNamesLocked` / `depSatisfiedLocked` / `entryDepsSatisfiedLocked`
  （按名依赖判定函数）。
- `core/lifecycle.go`：StatePending 注释更新（能力依赖）。
- `core/manager.go`：删除 `CheckCircularDependencies` / `declaredPluginDeps` / `topoSortPlugins`
  三个函数；新增 `resolvedDeps` 与 `backupSeq` 字段并在 Shutdown/Unload 路径同步清理；
  `LoadFromConfig` 重写——按能力依赖加载 + 反应式重算；agent primary LLM 改为从
  `m.resolvedDeps` 解析（`pickPrimaryLLM`）；注册 cron 工具族到 ToolRegistry。
- `core/sandbox_test.go`：TestSandboxWorkspaceAllowsRealPathCaseInsensitive 修复跳过判定
  + 新增 `isFileSystemCaseInsensitive` 探测函数。
- `main.go`：移除读 `agent.depends_on.llm` 选 primary LLM 的代码（改由宿主按能力依赖解析）。
- `plugins/tool-agentic-bench/README.md`：更新用例表（18 例）。
- `plugins/tool-agentic-bench/cases.go`：新增 cron_add_id 与 cron_list_roundtrip 用例。
- `plugins/tool-agentic-bench/main_test.go`：适配 18 用例计数 + 新增 3 个 cron 用例测试。
- `setup.go`：移除读 `agent.depends_on.llm` 选默认 LLM 的代码。
- `sdk/README.md`：声明式依赖章节重写——对齐 DSH/Cordis 的 provide + inject 模型。
- `sdk/dsc.go`：LLM gRPC plugin 改用 SDK metadataServer 融合 capabilities 与 requires。
- `sdk/metadata.go`：注释更新（不再提「写 depends_on」）；能力键前缀注释说明 provide 语义。
- `sdk/sdk.go`：Config.Requires 注释更新——对齐 DSH/Cordis 的 inject 模型，不再落盘。

### 新增
- `core/capability_deps.go`：能力依赖解析器（host 侧）。
- `core/capability_deps_test.go`：解析器测试（13 例）。
- `core/cron_tools.go`：CRON 工具族（cron_add/list/remove/set_enabled）。
- `core/cron_tools_test.go`：CRON 工具测试（8 例）。
- `sdk/metadata_test.go`：SDK metadata 编码测试（5 例）。
