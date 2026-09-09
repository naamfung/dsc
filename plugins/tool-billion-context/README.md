# tool-billion-context

DSC 上下文管理插件：以 [acp-kernel](https://github.com/ranxianglei/acp-kernel) 算法接管 DSC 默认的压缩机制。

## 设计

- **Go 原生重实现 acp-kernel 核心**（~800 行 Go，对齐 MIT 协议）
- **零 agent 改动**：经 SDK `Hook.OnEvent` 拦截 `agent/pre-step` 事件，改写消息列表
- **模型驱动压缩**：模型决定 *何时* 压、*压什么范围*，插件编排其他一切
- **3 层 LSM-tree**（MVP 仅实现 T1，T2/T3 留接口）
- **state 持久化**：每个 session 一个 JSON 文件（`ExecDir/billion-context/<session>.json`）

## 4 个模型可见工具

| 工具 | 用途 |
|------|------|
| `compress` | 把消息范围替换为模型生成的摘要 |
| `decompress` | 恢复压缩块内容 |
| `search_context` | 在压缩块摘要中搜索关键词 |
| `acp_status` | 查看上下文使用率与可压缩范围 |

## 接管流程

1. 插件加载时注册 4 个工具 + `Hook.OnEvent`
2. agent 发起 LLM 请求前，宿主 emit `agent/pre-step` 事件
3. 插件收到事件，经 `acp.ProcessTurn` 跑 pipeline：
   - 分配 `mNNNNN` ref 给每条消息
   - 注入 `<acp tokens="N">mNNNNN</acp>` 标签
   - 应用 prune（被压缩块覆盖的消息替换为 summary）
   - 决策是否注入 nudge 提示
4. 返回改写后的消息列表给宿主，宿主透传给 LLM
5. 模型调 `compress` 工具时，插件调 `acp.ApplyCompression` 持久化块

## 配置

环境变量：
- `DSC_CONTEXT_WINDOW`：模型上下文窗口大小（默认 131072）
- `DSC_EXEC_DIR`：state 存储目录（默认当前目录）
- `DSC_SESSION_ID`：当前会话 ID（默认 "default"）

## MVP 范围

已实现：
- T1 单层压缩
- 子串搜索（后续升级 BM25）
- state JSON 文件持久化
- agent/pre-step 拦截 + 消息改写
- 4 个工具 handler

留待后续：
- T2/T3 多层蒸馏
- absorb 即时工具结果吸收
- fork/恢复时 state 重建（经 session 事件日志重放）
- BM25 / fuzzy 搜索
