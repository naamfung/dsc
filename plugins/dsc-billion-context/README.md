# dsc-billion-context

DSC 上下文管理插件：以 [acp-kernel](https://github.com/ranxianglei/acp-kernel) 算法接管 DSC 默认的压缩机制。

## 设计

- **Go 原生重实现 acp-kernel 核心**（对齐 MIT 协议，独立重写非移植）
- **经 SDK 通用事件钩子接管**：`Hook.OnEvent` 拦截 `agent/pre-step` 事件改写消息列表；`Hook.ContextFn` 贡献 ACP system prompt（对齐 DSH `ctx.systemPrompt.section`）
- **模型驱动压缩**：模型决定 *何时* 压、*压什么范围*，插件编排其他一切
- **3 层 LSM-tree**：T1 标准压缩、T2 蒸馏（T1→T2）、T3 凝结（T2→T3）
- **前缀缓存稳定**：summary 原位嵌入（不堆到头部）、不注入 `<acp>` 标签到消息文本、nudge 作为尾部 user 消息
- **孤儿配对清理**：压缩边界切断 tool-call ↔ tool-result / reasoning ↔ assistant 时自动清理
- **state 持久化**：每个 session 一个 JSON 文件（`ExecDir/billion-context/<session>.json`）

## 4 个模型可见工具

| 工具 | 用途 |
|------|------|
| `compress` | 把消息范围替换为模型生成的摘要（T1）；用 `bN` 块引用触发蒸馏（T2/T3） |
| `decompress` | 恢复压缩块内容 |
| `search_context` | 在压缩块摘要中搜索关键词（子串匹配） |
| `acp_status` | 查看上下文使用率与可压缩范围 |

## 接管流程

1. 用户在 `config.yaml` 中声明 `compaction: dsc-billion-context` 显式选择后端
2. 宿主验证插件声明了 `Provides: {"compaction": "true"}` 能力（对齐 DSH preset + 能力验证）
3. 插件经 `Hook.ContextFn` 贡献 ACP system prompt（压缩哲学、何时压缩、工具使用说明、蒸馏规则）
4. agent 每轮 `buildSystemPrompt` 经 `ListContext` 拉取 ACP 指导（对齐 DSH `ctx.systemPrompt.section`）
5. agent 发起 LLM 请求前，宿主 emit `agent/pre-step` 事件
6. 插件收到事件，经 `acp.ProcessTurn` 跑 pipeline：
   - 分配 `mNNNNN` ref 给每条消息（FNV-1a 内容 hash 生成稳定 ID）
   - 应用 prune（被压缩块覆盖的消息替换为 summary，原位嵌入保持前缀稳定）
   - 清理孤儿 tool-call/result/reasoning 配对
   - 决策是否注入 nudge 提示（45% 阈值 + 增长门控 + T2/T3 蒸馏触发）
7. 返回改写后的消息列表给宿主，宿主透传给 LLM
8. 模型调 `compress` 工具时，插件从缓存的 `lastMessages` 解析 `mNNNNN` 边界执行压缩
9. 上下文溢出时，插件经 `agent/request-error` 事件触发紧急压缩后返回 `{"retry": true}`，宿主重新走 pre-step + 重新调 provider

## 前缀缓存稳定性

| 设计 | 说明 |
|------|------|
| summary 原位嵌入 | 每个 summary 放在它替换的原始范围的最早消息位置（`insertAt`），不堆到列表头部 |
| summary 位置稳定 | 经 `acp_summary_<blockId>` 前缀识别"已渲染的 summary"，下次 prune 时保持位置不变 |
| 不注入 `<acp>` 标签 | `renderTags=false`——标签的 tokens 属性每轮估算会变化，破坏前缀缓存 |
| nudge 尾部追加 | nudge 文本每次不同（含 usage%、ranges），作为尾部 user 消息，不影响前缀 |
| ACP system prompt 经 ListContext | 经 `Hook.ContextFn` 贡献，agent 的 `buildSystemPrompt` 经 `ListContext` 拉取，位置固定 |

## 多层蒸馏

| 层级 | 触发条件 | 输入 | 输出 |
|------|---------|------|------|
| T1 标准压缩 | 上下文用量 ≥ 45% + nudge | `mNNNNN` 消息范围 | ~2K token 结构化摘要 |
| T2 蒸馏 | active T1 块数 ≥ 5（`tier2Trigger`） | `bN` 块引用（如 `b0..b4`） | ~1K token 高阶聚合摘要 |
| T3 凝结 | active T2 块数 ≥ 10（`tier3Trigger`） | `bN` 块引用 | 30-60 token/块 超浓缩事实索引 |

模型在 `compress` 调用中用 `bN` 形式作为 `startId`/`endId` 触发蒸馏路径。

## 与 agent 内联压缩的关系

agent-react-loop 的内联 `compactHistory`（80% 阈值）作为**兜底安全网**保留：
- billion-context 在 45% 阈值主动 nudge 模型压缩，使 80% 正常不被触发
- agent 经 `ListContext` 响应中的 `[DSC_COMPACTION_BACKEND_ACTIVE]` 标记检测后端存在
- 后端卸载后标记消失，agent 自动恢复内联压缩
- 不依赖 env 或跨进程状态同步——`ListContext` 每轮实时反映插件生命周期

## 配置

在 `config.yaml` 中声明：
```yaml
compaction: dsc-billion-context
plugins:
  - name: dsc-billion-context
    type: dsc
    enabled: true
    binary_path: ./plugins/dsc-billion-context/dsc-billion-context
```

环境变量：
- `DSC_CONTEXT_WINDOW`：模型上下文窗口大小（默认 131072）
- `DSC_EXEC_DIR`：state 存储目录（默认当前目录）
- `DSC_SESSION_ID`：当前会话 ID（默认 "default"）

## 已实现

- T1/T2/T3 完整多层压缩与蒸馏
- 前缀缓存稳定（原位嵌入 + 不注入标签 + nudge 尾部追加）
- 孤儿 tool-call/result/reasoning 配对清理
- nudge 决策（阈值门 + 增长门 + baseline 重置 + T2/T3 计数触发）
- ACP system prompt 经 `Hook.ContextFn` 贡献（对齐 DSH `ctx.systemPrompt.section`）
- 稳定消息 ID（FNV-1a 内容 hash，不含视图 index）
- request-error 重试闭环（溢出紧急压缩 + `{"retry": true}`）
- state JSON 文件持久化
- 4 个工具 handler
- 18 个单元测试

## 留待后续

- absorb 即时工具结果吸收
- fork/恢复时 state 重建（经 session 事件日志重放）
- BM25 / fuzzy 搜索
- merge-blocks 节点（批量合并 old 块）
- hide-compress-calls（隐藏已消费 compress 调用）
- emergency-truncate（近满时截断大工具输出）
