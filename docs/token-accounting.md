# DSC 词元计量：算法、数据流与规范

> 面向后来者：读完这一篇应当能回答——某个数字是谁算的、什么时候算的、能不能信、
> 想改的时候该动哪里。文中所有公式与行为都以当前代码为准，并标注了文件与函数。

---

## 1. 先记住三个数

DSC 里与「token 用量」有关的数只有三类，**用途完全不同，不能互相替代**：

| 数 | 谁产生 | 精度 | 用途 |
|---|---|---|---|
| **估算（estimate）** | `core/token_meter.go` 的纯函数 | ±10%~30%（看内容） | 没有实测值时的绝对值；有实测值时的**差量** |
| **实测（usage）** | provider 上报的 `usage.prompt_tokens` | 精确（但只对"那一次请求"精确） | 压力底数、界面"已用" |
| **投影（projection）** | `TokenMeter.Project` / `core.projectPressure` | 实测底数 + 估算差量 | 决策：要不要裁剪/压缩 |

一句话：**服务端是唯一的绝对真值，估算负责"没有真值时"和"真值之后的增量"。**

---

## 2. 分层与唯一真源

```
core/token_meter.go        估算公式（唯一实现）+ TokenMeter
        ▲
sdk/tokenmeter.go          一行转发（插件唯一入口 dsc.Estimate* / dsc.TokenMeter）
        ▲
plugins/*                  只能调 dsc.*，不得自带公式
```

| 关注点 | 唯一实现位置 |
|---|---|
| 文本/消息/工具目录估算 | `core/token_meter.go`：`EstimateTextTokens`、`EstimateMessageTokens`、`EstimateMessagesTokens`、`EstimateProtoMessage*`、`EstimateToolTokens`、`EstimateProtoToolTokens` |
| 单个工具的估算公式 | `core/token_meter.go:estimateOneToolTokens`（core.Tool 与 proto.Tool 共用，两形态不得算出两个数） |
| 压力合成策略 | `core/llm_router.go:projectPressure`（宿主）+ `plugins/dsc-system` 的**自身表面**测量（见 §8） |
| 用量锚点倒查 | `session/usage.go:LastUsageAnchor` |
| 请求信封指纹 | `core/tool_fingerprint.go:ToolsFingerprint` |
| 缓存口径 | `core/token_meter.go:PromptSideTokens` |

**硬约束**：核心/SDK/前端不得出现插件名（`plugin_names_guard_test.go` 会扫）；插件不得
再写一份 `/4`、`max(bytes/4,runes)` 之类公式——历史上两份口径打架，CJK 低估 4 倍、
ASCII 高估 4~5 倍，压缩疯狂误触发。

---

## 3. 估算算法（局部）

### 3.1 文本：按字符分类累加

```go
// core/token_meter.go:EstimateTextTokens —— 按字符类别加权（千分整数，末尾一次性进位）
for _, r := range s { acc += tokenWeightPerMille(r) }
return (acc + 999) / 1000
```

| 类别 | 千分权重 | 每 token 约合 |
|---|---|---|
| ASCII 字母 | 226 | 4.4 字符 |
| 空白（空格/换行/制表） | 139 | 7.2 字符 |
| ASCII 数字 | 624 | 1.6 字符 |
| ASCII 标点与符号 | 734 | 1.4 字符 |
| CJK（表意/假名/谚文/全角） | 873 | 1.15 字符 |
| emoji | 1800 | 0.6 字符 |
| 其余非 ASCII（破折号/箭头/引号/带音标字母） | 1000 | 1 字符 |

这组数**不是估的**：把真实会话内容切成 3000 字符的片段，逐段发给模型端点读
`usage.prompt_tokens`，再对「各类字符数 → token 数」做最小二乘（36 段训练 / 12 段留出）。
留出集 pred/actual 中位 1.008、p10–p90 = 0.90–1.10；旧公式在同一批样本上中位 0.876
（系统性低估 12%，单段 0.74–1.12）。

旧口径「宽字符 1 token/字、其余 4 字节/token」的两个假设都被否证：

- **窄字符并不等价**：标点 0.73/字、数字 0.62/字，是字母（0.23）的 3 倍。工具输出、
  代码、JSON 全是这两类，按 0.25 估会低估 1.5–3 倍；
- **CJK 也不是 1 token/字**，实测 0.87（合成重复文本才会掉到 0.62——BPE 会合并）。

两处误差方向相反，在混合内容里互相抵消，所以旧公式「看起来还能用」；但会话一旦偏向一侧
（纯中文，或纯工具输出）就系统性偏掉。**偏差系数只能整体缩放，修不了这种「随内容构成
变化」的误差**——这正是分类权重的意义。

工具定义（§3.3）反其道而行：它是高度重复的同构 JSON，BPE 合并得很好，实测边际
0.257 token/字符，所以仍按「字节/4 + 11」计，不用内容系数（否则高估 2 倍以上）。

> DSH 用固定的 `CHARS_PER_TOKEN=4`，其 README 自认「CJK text and JSON schemas underprice
> badly」。我们保留 CJK 感知版，是有意为之，不是没对齐。

### 3.2 消息：文本 + 结构性开销

```go
// EstimateProtoMessageTokens
toks  = EstimateTextTokens(content)
toks += 6  // role=="tool"：tool 角色 + tool_call_id
    或 += 4  // 其余：role + 分隔符
toks += Σ imageStructuralTokens(ref)   // 4 + ceil(len(ref)/4)，图片只算引用、不算像素
toks += Σ (EstimateTextTokens(name+arguments_json) + 8)  // 每次工具调用
```

### 3.3 工具目录：每次请求都带的固定成本

```go
// estimateOneToolTokens —— 刻意不用内容分类权重
11 + (len(name)+len(description)+len(parameters_json)+3)/4
```

工具定义与消息内容是两种文本：它同构、重复、由 provider 紧凑序列化，BPE 合并率高。
实测边际 0.257 token/字符（schema 250 字符 → 75 token；2274 字符 → 595 token），
用内容系数（0.6–0.7/字符）会高估 2 倍以上。

**必须计入**。工具目录每次请求都发、且不随会话增长消失。本机实测：13 个插件 46 个工具
≈ **10,037 tokens**，占压缩后 ~42K 请求的 **24%**。漏算它等于压力永远少一截，窗口快满
仍判「还宽裕」。

因此任何"请求大小"都必须是：

```go
EstimateProtoMessagesTokens(msgs) + EstimateProtoToolTokens(tools)
```

宿主 pre-step 的 `TokenCount`、agent 的投影/单轮超窗/压缩预算、压缩后端自测表面都遵循此式。

#### 3.3.1 目录里放什么，本身就是一个预算决策

目录是**固定成本**，所以「哪些工具进目录」跟公式一样重要。三条已落地的判断：

- **MCP 默认关闭**：注入开关（`config.yaml` 的 `mcp.enabled`，由 `/mcp on|off` 写回，
  无此段即未启用）是总闸。关着时 MCP 相关工具**一个都不进目录——含入口 `mcp_connect`**，
  日常会话为它付 0 tokens。实测五个管理工具合计 1,087 tokens（connect 309 / list 243 /
  load 215 / disconnect 161 / unload 159），即开关关闭省下的就是这 1,087。开关打开后：
- **MCP 工具默认不进目录**，由模型 `mcp_load` 按需加载、`mcp_unload` 撤出（`core/mcp_tools.go`）。
  实测一台 229 个工具的服务器：全量进目录 **74,543 tokens**（1M 窗口 7%，128K 模型 **57%**），
  连上但已加载 0 个时只有 4,756（含 4 个管理工具）。按 server 配白名单也能降，但那是把
  「哪些工具该出现」固化成用户输入（改配置要重启、换任务又对不上）；按需加载把判断留在运行期。
  代价写清楚：每次 load/unload 都会改变目录 → 前缀缓存失效一次，所以**按需成批**，不是每步一抖。
- **没有 MCP 连接时，运行期管理工具自己也不进目录**（`mcp_list`/`mcp_load`/`mcp_unload`/
  `mcp_disconnect`，合计 778 tokens）：它们一个也用不上，留着就是纯成本。入口 `mcp_connect`
  常驻——它是把「没有」变成「有」的唯一手段。

裁定点只有一个：`Manager.AllToolsProto()` 末尾的 `filterMCPCatalog`。它同时供主 agent 的
tools 数组、子代理的 tools 数组、`run_code` 的 SDK 清单——放在任何**单一消费者**旁边
（例如 agent 插件里过滤）都只能照顾到一路，另两路照旧收到整份目录。

### 3.4 校准：把每一次真实请求变成样本

系数是近似，provider 的计数是真值。两者的比值有记忆价值，所以每次结算都落两处：

| 文件 | 内容 | 用途 |
|---|---|---|
| `<sessions>/<会话键>.usage.jsonl` | 逐请求：`sent`（最终发出表面的估算）、`actual`（`prompt_tokens`）、消息数、工具目录估算、**七类字符计数** | 审计 + 重拟合的原始数据 |
| `<sessions>/token-calibration.json` | 按模型：`factor`（EWMA 比值）、`samples`、`last`、`spread`（p90/p10）、`updated_at` | 冷启动的**唯一**依据 |

三点约束：

- **`sent` 必须是最终发出表面**（pre-step 改写之后）的估算，与判超窗同源。日志里的
  `request/header.SentEstimate` 是 agent 组装时的估算（压缩**前**），与实测相除得到的是
  压缩比（实测中位 0.194），不是偏差——不要拿它当校准样本。
- **校准是跨会话、跨重启的**。新会话/冷启动没有锚点时，`estimateInflation` 用校准表而不是 1；
  校准表丢了（首次升级、误删）就从账本重建，不必从 1 重新学。
- **换模型先借后换**：内存锚点按模型分桶，当前模型没有记录时借最近一份，本帧结算即替换。

**漂移检测**：连续 3 帧与当前 `factor` 偏离 >15% 记一条 warn（单帧偏离不算——整段 base64
这种内容本来就会让比值跳一下）。`spread`（窗口内 p90/p10）变大意味着**内容构成在变**：
整体比例还够用，但该重拟合系数了。

**重拟合是自动的**（`core.TokenCoefSet` / `FitTokenCoefficients`）：

- 触发：进程启动时若账本够样本就拟一次；运行期每满 500 帧再后台拟一次。
- 门槛（过不了就保持原样）：样本 ≥200 帧；拟合残差必须**优于**现用系数；权重越界即弃。
- 某类字符总量 <5000 时**该列保留原值**，不整份拒绝——emoji/生僻符号多数账本里没有，
  「缺一类就拒绝」等于永远拟合不出东西。
- 结果写 `token-coefficients.json`（按模型各存一份，含 `active_model`）。

**生效时机：下一个进程**。系数是进程级全局量（宿主判超窗、插件算压缩预算必须同一套），
运行期改动会让已启动的插件与宿主各用一套——「按 A 判、按 B 学」。所以宿主在**拉起插件时**
把当场生效的那套写进 `DSC_TOKEN_COEF`（`coreEnv` 注入，SDK `init` 读取），运行期不再变。

```bash
tokcal -dir <sessions>              # 看：每模型样本量、比值、漂移、拟合结果与拒绝原因
tokcal -dir <sessions> -write       # 立即写盘（仍要下次启动生效）
```

工具与自动流程共用 `core.FitTokenCoefficients`，所以「看到的结果」和「实际会用的结果」
是同一份实现。它额外标出**不可信的列**（负值/超界/该列字符数过少，通常是共线，例如
中文与全角标点总同现）——那种情况下预测仍然准，但单个权重没有意义。

---

## 4. 计量器 `TokenMeter`

```go
type TokenMeter struct {
    pressureTokens  int   // 实测：上一次成功请求的 prompt 侧用量
    sampledEstimate int   // 取锚点那一刻，对所发请求（消息+工具）的估算
    hasSample       bool
    contextWindow   int
}
```

两种建锚方式，语义不同：

| 方法 | 设定 `sampledEstimate` | 之后 `Project(x)` |
|---|---|---|
| `SampleUsage(usage, sentEstimate)` | 有（必须与该次请求同源） | `usage + (x − sentEstimate)`，即**实测底数 + 差量** |
| `UpdateUsage(usage)` | 0（不设基线） | **退回 `x`**（实测值只描述那一次请求，不能当投影） |

`Project` 的差量**可以为负**：压缩让表面变小时，"已用"要跟着回落，否则永远在喊"该压缩了"。
`ShouldCompact(currentEstimate, ratio)` 只在 `window>0` 时给出判断。

**没有增量基线时绝不能拿实测值当投影**（`hasSample && sampledEstimate <= 0 → x`）。
实测值描述的是"那一次请求"，`x` 描述的是"即将发出的这一发"，两者之间没有可换算的关系。
曾经这里返回实测值当"保守下界"，在小窗口读超长会话（滑动窗口，每步都在裁剪）时就是
界面卡死：裁剪后不重新锚定 → 基线永远是 0 → `Project` 恒等于启动倒查到的旧用量。
实测现场是会话已到 730K（provider 实测），界面「已用」从首帧起一直是 33K / 3.29%，
而速率、缓存命中照常刷新（它们读的是另一个数）。

所以还有两条配套约束：

- **`UpdateUsage` 的实测值只用于 `MeasuredPromptTokens()`**（展示兜底、锚点超窗判断），
  不参与投影换算。
- **锚定必须无条件做**：agent 侧 `SampleUsage(promptTokens, sentEstimate)` 不再因为
  "本步裁剪过"跳过。跳过就等于永远拿不到基线。前提是投影输入也换成**裁剪后真正发出的
  表面**（否则基线与投影不同源，差量会把被裁掉的那段算成增长）。
- **展示兜底读 `MeasuredPromptTokens()`，不读 `Project(0)`**：后者算的是
  `实测 − 该次请求的估算`，即启发式的**误差**（几万量级、几乎不随会话增长），
  不是规模。

**锚点自身超窗时退回纯估算**（`window>0 && pressureTokens > window`）：上一次实测已经
大于窗口，说明窗口配置（或探测值）小于 provider 实际接受的规模。继续以它为底数，会把
之后每一次请求都判成超窗——历史裁到只剩最近几条也照样报错，而 provider 明明一直在接收。
这种锚点不再描述"现在要发的请求"，判定退回 `currentEstimate`。窗口未知（0）时无从比较，
维持差量语义。

`PromptSideTokens` 返回 **prompt_tokens**，不叠加 `cacheRead/cacheWrite`：OpenAI 兼容端点
的 prompt 已含缓存命中，相加会把命中缓存的请求算成近两倍（历史 bug，commit `1de5494`）。

---

## 5. 信封与锚点：让"上一次用量"变得可比

用量只对**产生它的那个请求**有效。换了模型、窗口、工具目录，或上一次请求被压缩改写，
那个数字就不再描述现在要发的表面。DSC 用一个独立事件记录这点。

### 5.1 `request/header`（log-only，请求发出前写）

```go
type RequestHeaderData struct {
    Turn, Step    int
    ToolsHash     string // core.ToolsFingerprint
    Model         string // 宿主经 DSC_MODEL 注入
    ContextWindow int
    SentEstimate  int    // 对该请求（消息+工具）的启发式估算
}
```

- 在 agent 构造完 `ChatRequest` 后、真正调用前 `Append`（`plugins/agent-react-loop`）。
- **失败请求也留下信封**（诊断用）；不进 surface、不派生模型消息。
- 新增事件类型**必须**同时在 `session/persist.go` 的解码表注册，否则加载会话时报
  `unknown event type`（写的时候不报、读的时候炸）。

### 5.2 `llm/attempt`（log-only，结算时写）

只记结果：`Usage` / `Error` / `FinishReason` / 耗时 / 产出规模。**不含信封**——同一事实
不存两处。

### 5.3 配对与可比性

```go
// session/usage.go:LastUsageAnchor
// 倒查最近一条「成功且 prompt>0」的 attempt，再向前找同 Turn/Step 的**最近一条** header
```

同一步重试会写多条 header，取最近那条才是这次尝试实际用的信封。之后：

```go
// UsageAnchor.Comparable
ToolsHash 一致  &&  模型一致（任一侧为空则跳过该条件） &&  窗口一致（0 视为未知）
```

加一条实测值的可信度判据（见 §7）：`usage >= SentEstimate`。低于它说明那次请求被压缩
或改写，用量描述的不是这个表面。

**旧日志没有 header → ToolsHash 为空 → 恒不可比**。这是刻意的保守方向：宁可退回估算，
也不拿不可比的 42K 当 900K 表面的底数。

### 5.4 指纹 `ToolsFingerprint`

按名称排序后，逐个写入 `name\0description\0parameters_json\0\x01`，取 sha256 前 16 hex。
顺序无关、任一字段变化即变。空工具目录返回空串（= 无信封信息，判不可比）。

---

## 6. 宿主投影 `projectPressure`

```go
// core/llm_router.go
projected, matched := projectPressure(anchor, curEstimate, curToolsHash, curModel, window)
```

三分支（`curEstimate = 消息 + 工具目录`）：

1. **信封不可比**（指纹/模型/窗口不同，或无 header）→ `curEstimate`
2. **可比但 `anchor.usage < anchor.sentEstimate`**（那次请求被改写）→ `curEstimate`
3. **可比且用量不低于该信封估值** → `anchor.usage + (curEstimate − anchor.sentEstimate)`

结果随 `agent/pre-step` 事件下发：

| 字段 | 含义 |
|---|---|
| `token_count` | 原始请求表面（消息 + 工具）的估算 |
| `last_usage_tokens` | 上一次成功的实测 prompt 用量（0 = 无） |
| `projected_tokens` | 上述三分支的结果，**针对"原始请求表面"** |
| `envelope_matched` | 锚点与本次请求是否同一信封 |

---

## 7. 表面所有权：`projected_tokens` 不是万能的

这是本文档最重要的一条规范：

> **压力必须量在那个"真正会被发出去"的表面上。谁能改写表面，谁就必须自己量。**

`projected_tokens` 描述的是**原始请求表面**（agent 派生出的完整历史 + 工具）。它对
"不改写请求"的消费者是精确的；但对**会改写请求的压缩后端**是错的——因为它压缩之后
只发「摘要 + 保留尾部」，而宿主看不到这次改写。

实测代价（`dsc-system`，窗口 10k、阈值 8k、保留 16%）：

| 场景 | 用 `projected_tokens`（原始 19k） | 自测渲染后表面 |
|---|---|---|
| 首次（尚无压缩记录） | 压缩 ✅ | 压缩 ✅ |
| 尾部只增 2 条（渲染后 ≈2.4k） | **又摘要一次** ❌ | 不新增摘要 ✅ |
| 尾部增到渲染后 >8k | 压缩 ✅ | 压缩 ✅ |

也就是说：直接用投影会把 80% 阈值退化成「保留区一满就压」，每步多打一次 LLM。

所以 `dsc-system` 的规则是：

```go
switch {
case len(records) > 0:  // 表面已被我们改写
    pressure = EstimateProtoMessagesTokens(rewritten) + toolTokens
case ev.ProjectedTokens > 0: // 尚未改写，表面 == 请求表面
    pressure = ev.ProjectedTokens
default:
    pressure = ev.TokenCount
}
```

`toolTokens`：优先用 `ev.ToolTokens`（宿主直接给出）；只有旧宿主没带该字段时才反推
`ev.TokenCount − EstimateProtoMessagesTokens(msgs)`。反推只作兜底——同一条瀑布里若别的
插件先改写过消息，这个减法会把差额算成工具开销，甚至算成负数。

另一个后端 `dsc-billion-context` 同理但更极端：它把绝大部分历史遮蔽成引用，真实载荷是
渲染后的视图（1M 窗口的真实会话实测 765K vs 原始 1.66M；早期样本 40K vs 900K），因此它用
`last_usage_tokens`（上一次真正发出去的量），**不用** `projected_tokens`，代码注释里写明
理由，防止后来者"顺手统一"。

### 7.1 agent 不做请求级裁剪（与有没有后端无关）

承接上一条「谁能改写表面，谁就必须自己量」：**agent 手里的投影不描述将要发出的东西**，
任何基于它的裁剪都是瞎猜——有压缩后端时是这样（后端会把表面积改小），没有后端时也一样
（真正决定能不能发出去的是宿主收口层，§7.2）。

`TokenMeter.Project` 的锚点一头是**改写后**的 usage（比如 169K），一头是**改写前**的
估算（比如 932K）——两者描述的是两个不同的列表。`usage + (x − sentEstimate)` 于是把
后端省下的量当成「历史缩水」，投影在相邻两步之间来回摆：实测同一会话里 agent 的发送面
在 932K / 610K 交替（对应 provider 的 169K / 26.5K），裁剪深度也跟着每步翻转，前缀缓存
每次全废。

因此裁定（`plugins/agent-react-loop` 的 step 循环）：agent **只做循环与调度**，

- **不裁剪**（没有任何 `trimToWindow` / 调用）；
- **不折叠**（不改写任何消息正文；`foldOversizeMessages` 已删除）；
- **不做摘要化压缩**（`compactHistory` 已删除；压缩是声明 `Provides compaction` 的插件的
  职责，agent metadata 用 `requires/dsc/compaction` 声明这个强依赖）；
- **不做「单轮内容超出上下文窗口」硬报错**：它量的是改写前的列表，而真正上线的是后端渲染
  结果（实测 765K vs 原始 1.66M）或宿主滑窗后的视图，拿前者判后者会把完全装得下的请求判死。

窗口适配的两条路径都在宿主：改写后的收口（§7.2）与 provider 4xx 后的滑窗重试
（`core/overflow_adjust.go`）。没有当值压缩后端时进程根本不会启动（agent 依赖未满足）。

### 7.2 发出去的那一份必须有硬上限（宿主侧收口）

§7.1 的「agent 不裁剪」有个必须补的另一半：**谁保证最终发出去的 ≤ 窗口？** 谁都没有——
后端渲染多大由它自己决定，而它并没有「必须塞进窗口」的义务。真实事故：1M 窗口的会话，
后端把 1.66M 历史渲染成 1.07M 就发了，provider 回
`prompt is too long: 1075596 tokens > 1048576 maximum`，整轮失败；`request-error` 的
重试确实触发了，但每次紧急压缩只压一个范围，3 次重试仍没压到窗口以内。

所以在 pre-step 改写**之后**、请求发出之前，宿主统一收口（`core/context_ceiling.go`，
`finishPreStep`）：

1. 量最终表面（消息 + 工具目录）。装得下就**一字不改**（零成本、前缀缓存不动）；
2. 装不下就**滑窗**：从头丢到「真实估算 × 偏差系数」落进 `比例 × 窗口`。比例按证据强度选：

   | 情形 | 比例 | 原因 |
   |---|---|---|
   | 本会话已有成功实测 | 0.90 | 偏差系数已可信，余量够 |
   | 本会话还没有任何实测（续长会话的首帧/冷启动） | **0.80** | 那一刻最没依据：实测 939,014 估算 → provider 1,099,278（差 17%），10% 余量不够 |
   | 被 provider 拒过 1 / 2 / ≥3 次 | 0.78 / 0.68 / 0.60 | 单靠 inflation 不收敛：越裁越旧的越轻、留下的尾部越密，同一比例在裁完后的表面上又低估一截 |

   首条 system 保留，
   保留段不以 tool 结果开头，至少保留最后一条。

**为什么超窗不先请后端紧急压缩**（曾经这么做过，已撤回）：

- 紧急路径只能产出**没有内容的占位摘要**：一类后端写「N 条消息已压缩」的固定模板，
  另一类 `emergency=true` 强制走截断摘要、直接跳过 LLM（两边都不调模型，所以「超窗就
  发不出去、模型收不到」并不成立——但价值也不成立）。一次紧急压缩就把几百条真实消息
  永久埋进一句空话，而滑窗只影响这一次请求：会话日志一字不动，下一轮仍可正常压缩。
- 一次滑窗**必然**装得下；「压一个范围再重试」不保证收敛——1.07M 那次连续三次重试都没
  压进窗口，最后整轮失败。
- provider 的 **4xx 也走滑窗**，不再派给插件做有损的紧急压缩：宿主从错误文本里取
  「实际用量 / 上限」（`prompt is too long: 1075596 tokens > 1048576 maximum` 这类），
  把偏差记进本会话预算（`core/overflow_adjust.go`），然后**重试**——重试时收口按收紧
  后的预算滑窗。连续数次取更严的值；一次成功即清除（一次异常不该让会话永远按放大系数裁）。

一句话：**超窗是宿主的收口责任（确定性滑窗），压缩是后端在自己阈值内的正常职责**，
两者不要互相代劳。

**"自量优先"是硬要求**：正常路径完全靠本地测量（消息 + 工具目录，含偏差系数）决定发不发；
provider 的 4xx 只作为「估算失真」的兜底，不是发现超窗的手段。任何「先发出去看看会不会
报错」的实现都属于把成本转嫁给用户的一轮失败。

**滑窗的不变量：窗口里除开头那一行「切除说明」，每条都是原始消息。**

- 实现上就是「按指针切一段」：`CapMessagesToWindow` 返回的是同一批 `*proto.Message`
  的子切片（首条 system + 保留段），内容、附件、工具调用一字不改。滑窗的语义是「换一个
  起点重新读同一份历史」，不是「把历史改小」。折叠 / 截断 / 摘要都属于**压缩**，不属于滑窗。
- 窗口开头插一条独立的 `system` 消息：「[滑窗] 已省略更早的 N 条消息（约 M tokens）；
  会话记录仍完整，以下为窗口内的原始消息。」——它不替换、不修饰任何原文。少了这一行，
  模型会把「当前这段的开头」当成整个对话的开头。
- 丢掉的是**最旧的一段**，最后一条永远保留；装不下就继续往前丢。
- 丢到只剩「首条 system + 最后一条」仍装不下，说明是**这一轮本身**太大：宿主在本地直接
  报错（说明多少 tokens / 窗口多大 / 该缩短什么），**不截断原文、不发出去换 400**。

**偏差系数**（`estimateInflation`）是这次事故的另一半成因的修正：锚点里同时存着同一请求的
`sentEstimate`（我们算的）与 `promptTokens`（provider 报的），两者相除就是这类内容的系统性
低估倍数。判定上限时用它把估算放大（上限 1.5×），而不是继续相信自己的算数。只放大不缩小：
偏大只会白丢历史。

这个系数**不随工具目录、窗口、模型变化而作废**。估算偏差是量级问题（中文 + 工具负载实测
常是 1.4–1.7×），换分词器只让比例移动一两成；而工具目录每次升级都变、窗口每次调参都变。
按「变了就作废」，重启后的第一条请求永远退回 1，而那恰恰是历史最厚、最没有新实测可学的
时刻——一次 98MB 会话重启后就这样把 92.7 万的估算发成 126.9 万，直接 400。取舍是不对称的：
系数偏大只多丢历史，偏小就整轮失败。

**换模型时不退回 1，而是先借上一个模型的比例**：锚点按模型分桶（`sessionAnchorSet`），
当前模型有记录就用自己的，没有就取最近一次别的模型的那份（`anchorForModel` 的 borrowed），
等本帧用量结算后自动换成自己的。借来的比例通常只差一两成，而退回 1 是拿「已知最危险的
未知」去赌一次可能撞墙的发送。切回旧模型时，它自己的比例还在。

裁剪在宿主侧重新变得安全，正是因为 §7.1 那条：**消息身份是宿主下发的稳定 id**
（`event-<seq>`），丢掉头部不会让压缩块的 `mNNNNN` 指到别的消息——那才是当初取消裁剪的原因。

配套硬约束：**消息身份必须是宿主下发的稳定 id**（`proto.Message.id`，派生时取 surface
事件的 seq）。后端的压缩块用 `mNNNNN` 指认消息，而 `mNNNNN` 由消息 id 映射而来；一旦
用列表下标当 id，`history_injection` 只带最近 N 条、fork、或任何裁剪都会让同一个引用
指到另一条消息——不是「压得不准」，是**遮错内容**（实测最坏时整段对话只剩 2 条）。

状态文件带 `schema` 版本，与当前不符即整体丢弃、由事件日志重建（`compress` 调用自带
范围与摘要）。不写逐字段迁移：旧格式里记的是位置式 id，硬迁等于给已废弃的寻址方式
留实现分支（AGENTS.md §4）。

---

## 8. 数据流全景

```
① agent 组装请求
   msgs = sess.DeriveMessagesLimited(historyInjection)
   sentEstimate = Estimate(msgs) + EstimateTools(tools)
   sess.Append(request/header, {Turn,Step,ToolsHash,Model,Window,SentEstimate})

② 宿主 pre-step（每次请求都会发生）
   tokenCount = Estimate(msgs) + EstimateTools(tools)
   anchor     = 该会话上一次成功请求的 {usage, sentEstimate, ToolsHash, Model, Window}
   projected  = projectPressure(anchor, tokenCount, hash(tools), model, window)
   → 广播 agent/pre-step{messages_json, token_count, tool_tokens, last_usage_tokens,
                          projected_tokens, envelope_matched}
   → 压缩后端（waterfall）改写 messages 或返回空（不改写）

③ 宿主发出请求（改写后的 messages + tools）→ provider

④ 结算
   宿主：recordSessionAnchor{usage, Estimate(改写后 req), hash, model, window}
   agent：sess.Append(llm/attempt, {Usage, Error, ...})

⑤ 下一轮回到 ②，此时 anchor 来自 ④（或重启后由 session.LastUsageAnchor 从日志恢复）
```

三个消费者读不同的数：

| 消费者 | 读什么 | 为什么 |
|---|---|---|
| agent（上报「已用」） | 自己的 `TokenMeter`（`SampleUsage` + `Project`） | 它只上报投影给前端；**不做**裁剪/压缩决策（那是宿主与压缩后端的事） |
| 宿主 pre-step | `projectPressure` | 给插件一个统一的、含工具目录与信封判据的投影 |
| `dsc-system` | 无记录时用 `projected_tokens`，有记录时自测 `rewritten` | §7 表面所有权 |
| `dsc-billion-context` | `last_usage_tokens` | 它的真实载荷是自己渲染出的视图 |
| 宿主 TUI | `HostContextMeters.ContextTokens`（agent 随帧上报的投影） | 只保管与发布，不重算 |

### 8.1 谁在什么时候动历史（分层与顺序）

多轮迭代后最容易搞混的就是这件事。按「从不可变到最激进」排列：

| # | 层 | 位置 | 触发条件 | 做什么 | 会话日志 |
|---|---|---|---|---|---|
| 0 | 会话日志 | `session/`（`*.jsonl`） | 每次事件 | 只追加。**任何裁剪都不改它** | — |
| 1 | 派生 | `session.DeriveMessagesLimited(n)` | 每次请求 | 事件 → 消息；`n` 只限制**从存档恢复**的历史（`config.history_injection`），当前进程内的轮次不受影响 | 不改 |
| 2 | 压缩后端改写 | `agent/pre-step`：`dsc-system`(compaction-basic) 或 `dsc-billion-context` | 每次请求 | 按自己的策略渲染（块遮蔽 / 保留尾部 / 摘要注入）。事件仍广播给所有插件，但**只有岗位持有者的改写被采纳**，也只有它收到完整消息列表（§8.2） | 不改会话日志；各自写自己的旁挂状态（`<id>.dsc-system.json` 摘要缓存 / `<id>.acp.json` 块与引用） |
| 3 | 宿主收口 | `core/llm_router.go finishPreStep` + `core/context_ceiling.go` + `core/overflow_adjust.go` | 每次请求（改写之后）；以及 provider 4xx 的重试 | 量最终表面；超窗从头部丢到「证据档比例」窗口（见 §7 的比例表；保留段全是原始消息 + 一行切除说明）；单条本身超窗则本地报错（不截断、不靠 400）。4xx 时用 provider 报的实际值收紧预算、**把这次被拒也记成校准样本**（只有总量没有构成，标 `no_classes` 只进比例不进系数拟合），并按失败次数逐次压低目标比例后重试 | 不改 |
| 4 | 展示口径 | `HostContextMeters`（`NoteActualSurface` / `ContextTokens`） | 每次请求 | 把「实际发出去的大小」发布给 TUI；不是另算一份 | 不改 |

**agent 不在这张表里。** 它派生历史后原样下发：不裁剪、不折叠、不做摘要。理由不是
消息身份（身份已是宿主下发的稳定 id，裁掉头部不会让压缩块指错消息），而是三条：

1. 职责：改写请求面归压缩后端，窗口适配归宿主收口层，agent 只调度；
2. 观测点错位：它手里是**改写前**的列表（可能几倍于窗口），真正上线的是后端渲染后 /
   宿主滑窗后的视图——拿前者决定丢什么，等于替后端扔材料；
3. 压缩是增量的（`dsc-system` 按已压前缀的 sha256 指纹复用摘要、billion-context 的块覆盖
   按它见过的历史建立）：喂它每步都被裁过的列表，前缀指纹每步失配 → 每步重算压缩，
   请求面在相邻两步之间摆动、前缀缓存反复失效（早期实测 169K/26.5K 交替，那时还叠加着
   按位置寻址的错位；那一层已随稳定 id 消失，增量这一层仍在）。

而「滑窗窗口内每条都是原始消息」的不变量只能由宿主收口那一层保证。

几条由此推出的结论：

- **同时只有一条改写路径在干活**：先是 2（压缩后端，可能改写），再是 3（宿主滑窗兜底）。
  「为什么我看到的裁剪数变了」先确认 `compaction` 配的是哪个后端、它是否真的加载了
  （`CompactionBackendState`）。
- **只有压缩后端可能写回会话**（摘要化不可逆），其余都只影响「这一次请求」。所以
  「切回大窗口还能看到原文」成立的前提是那段没被压缩过——滑窗丢掉的只是本次请求的
  视图，会话日志一字未改。
- **前端不输出任何裁剪/压缩提示**：滑动窗口是本产品的常态工作方式，不是要用户处理的
  异常。要看诊断就开 `-log`，那里有 `llm request: trimmed to fit the context window`
  这类记录。

### 8.2 压缩岗位：有能力者可以多个，当值者只有一个

「谁在改写请求面」不是每步去猜，而是启动期就定下来的**一个岗位**（`core/notify.go`
`ResolveCompactionRole` → `pickCompactionProviderLocked`）：

1. `config.yaml` 的 `compaction` 点了名 → 该插件当值（前提：已加载 + 声明了
   `Provides: {"compaction": "true"}`）；
2. 没点名、但**恰好一个**候选 → 它自动当值（启用即生效，用户不必再写一行配置）；
3. 没点名、多个候选 → **空缺**并要求点名（两个后端同时改写会层层叠加，结果既不是 A
   也不是 B），原因写在 `Vacancy` 里。

**空缺不再是正常形态**：压缩是 agent 的强依赖（agent metadata 声明
`requires/dsc/compaction`），它自己不实现压缩。没有当值后端时 agent 不会被激活，
`main.go` 在加载完成后据 `CompactionRoleProblem()` 直接 `fail`，把缺的是哪一环说清楚。

**载荷跟着岗位走。** `agent/pre-step` 仍派发给所有插件（副作用不能丢：重复调用链重置、
记忆落盘），但完整消息列表只发给岗位持有者：

- 岗位之外**没有任何插件能改写请求面**，把几 MB 的历史复制过去是纯白工——实测非岗位的
  压缩后端还会真的调模型做摘要，结果被宿主丢弃；
- 其余插件收到同一信封但不带 `messages_json`（轻载荷），`session` / `user_input` / 各计数
  都在，副作用需要的正是这些；
- 观察型插件若确实要看消息，声明 `Provides: {"pre-step-messages": "true"}` 即恢复完整载荷
  ——默认省流量，但需要的人有办法声明，而不是靠猜。

**`agent/request-error` 只发给岗位持有者**：只有它会做紧急压缩。广播给其他声明了
`compaction` 的插件，它们会真的再压一遍（调模型、写状态），结果同样被丢弃。

### 8.3 两个阶段：结构改写（唯一）与请求面投影（可叠加）

流水线是两段，不是一段：

| 阶段（`stage`） | 谁参与 | 结果 |
|---|---|---|
| `compaction` | 全体派发（副作用照旧），只有岗位持有者的改写被采纳 | 结构改写（块遮蔽 / 摘要 + 尾部） |
| `projection` | 声明了 `pre-step-transform` 的插件，按装载顺序 | 无损变换（按预算退役旧图像等），作用在上一阶段的视图上 |

分开的理由：压缩是「谁当值谁说了算」，投影是「可以叠加的若干道工序」。早先投影只能寄生
在压缩插件内部（插件内串链 `compaction → image-offload`），后果是**压缩换人时投影跟着
消失**——配了另一个压缩后端时，图像预算卸载等于从未存在。那不是配置问题，是把两件事绑
在了一起。

配套约定：

- 投影必须**无损**：不删消息、不改角色顺序、不动 tool 配对，只改单条消息的附件/正文。
  否则它无法安全叠加在压缩结果之上。`tools/backendcheck` 会按 `pre-step-transform` 能力
  再跑一次投影探针（图像超预算的夹具），校验这几点。
- 投影失败**不 veto**：它省的是字节，不是把请求压进窗口的那一步；报错记 WARN 并沿用上一步
  的视图。压缩失败则必须 veto（否则整轮必 400）。
- 岗位空缺（没配后端）时投影照跑：它的触发条件是预算，不是岗位。

两个插件侧配套（缺了会留下隐患）：收到空载荷必须**什么都不做**——尤其不能清掉
「最近一次消息列表」缓存（那是 `compress` 与紧急压缩的材料），也不能建/写 state。


---

## 9. 规范清单

**必须**

- 估算只用 `core/token_meter.go` 的公式，插件经 `dsc.Estimate*`。
- 「请求大小」= 消息 + 工具目录，任何压力/裁剪/预算都不得漏工具。
- **裁剪/压缩/折叠只记日志，不输出到前端**（`agent.logger` / 宿主 `m.logger`，`-log` 可见）：
  这些是常态运行状态，不是需要用户处理的异常；打到消息区只会把对话刷成同一句话。
- **"自量"是主路径，4xx 只是兜底**：每次请求前按「消息 + 工具目录（含偏差系数）」自测，
  超了就在本地滑窗；不要用发出请求→看 provider 报错的方式判断超窗。4xx 到来时也只
  收紧预算再滑窗重试，不派给插件做有损的紧急压缩。
- **滑窗不得改写消息正文**：窗口里除开头一行「切除说明」外，每条都必须是原始消息（同一对象、
  内容不变）。要"改小"某条消息属于压缩，必须走压缩路径并显式标注，不能伪装成滑窗。
- **发出去的请求必须有硬上限**：pre-step 改写后的表面超窗时，必须由宿主收口（§7.2），
  不允许「渲染多大就发多大」。后端没有「必须塞进窗口」的义务，provider 的 400 不是可接受的
  检测手段。
- **工具目录的裁剪只准在 `AllToolsProto()` 出口做**（`filterMCPCatalog`，见 §3.3.1）：
  它同时供主 agent、子代理、`run_code` 的 SDK 清单。在单一消费者旁边过滤会漏掉另外两路，
  而那两路照旧把整份目录发出去——MCP 一台服务器就可能是 7 万 tokens。
- **消息身份 = 宿主下发的稳定 id**（`proto.Message.id`，派生时取 surface 事件 seq）。
  任何跨请求指认同一条消息的东西（压缩块的覆盖集合、`mNNNNN` 映射）都用它；列表下标
  不是身份，只是位置（§7.1）。
- **压缩改写权归岗位**：`ResolveCompactionRole()` 是唯一解析点（配置点名且合格者当值；
  未点名时唯一候选自动当值；多候选未点名即空缺并要求点名）。压缩是 agent 的**强依赖**
  （metadata 声明 `requires/dsc/compaction`）：岗位空缺时 agent 不会被激活，宿主启动即失败。`agent/pre-step` 的完整消息列表只发给岗位持有者，`agent/request-error`
  只发给它——岗位之外没人能改写请求面，发过去就是白干。需要看消息的观察型插件用
  `pre-step-messages` 声明，不要在核心里按名字特判。
- **压缩（唯一）与投影（可叠加）分阶段**：`stage=compaction` 只采纳岗位持有者的结果；
  `stage=projection` 由声明 `pre-step-transform` 的插件依次作用在压缩后的视图上，必须无损、
  失败不 veto。**不要把投影塞回压缩插件内部**——那会让压缩换人时投影一起消失（§8.3）。
- 压力必须量在"将要发出的表面"上（§7）；改写者自测。
- **超窗判定只在宿主一处**：agent 不判定、不裁剪（见 §7.1）。宿主收口用同一份
  「消息 + 工具目录」的投影量与裁，且裁完要按同一口径复核——估算与投影分叉时一次裁不足。
- **压缩提示词必须命令模型在摘要里保留用户「当前最紧要任务/事项」**：`dsc-system`
  的 `llmSummary` 与 `dsc-billion-context` 的 `compress` 描述都写明——压缩后模型仍须知道
  现在要做什么、卡在哪、下一步是什么。当前任务靠模型自己的压缩选择保留，不靠程序钉死某条。
- **展示口径必须跟随「真正发出的表面」**：agent 的 `Usage.ContextTokens` 按
  `msgs + tools` 计算（`sentSurface`），而 `msgs` 是派生后原样下发的那一份；宿主侧若做了
  滑窗，用 `HostContextMeters.NoteActualSurface` 覆盖为实际发出量。
- **没有增量基线的实测值不得当投影**（见 §4）：它会冻结在界面上。锚定必须无条件做，
  且投影输入与锚定基线同源（都是发出去的那份表面）。
- **展示值一律夹在当前窗口内**：超过容量的部分发不出去（agent 会先裁掉），显示「714K/128K」
  只是在描述一个永远不会被发出的请求。这条对**启动预置**尤其要紧——预置值来自上一次请求，
  而那次可能跑在 1M 云模型上，这次换成了 128K 本地模型；且 `Seed` 发生在窗口已知之前
  （`LoadHistoryReplay` 先跑，`tui.New` 才 `SetContextWindow`），所以夹取必须在读取时做。
- 新增会话事件：`EventType` 常量 + `Data` 结构 + `session/persist.go` 解码注册，三处齐。
- 用量锚点只能通过 `session.LastUsageAnchor` 取（宿主与 agent 共用一份）。
- log-only 事件必须以 `Append(..., nil)` 写入（不进 surface）。

**禁止**

- 在插件里写 `len/4`、`max(bytes/4,runes)` 之类公式。
- 把 `llm/attempt` 当信封用，或把信封字段塞回 `llm/attempt`。
- 用 `projected_tokens` 代替"自己渲染后的表面"（会压过头）。
- 拿 `Project(0)` 当「已用」展示值：它算的是 `实测 − 该次请求的估算`（启发式误差），
  不是规模，量级几万且几乎不随会话增长——界面会永远停在一个假数字上。
- 用 `UpdateUsage(usage)` 的实测值当投影（无基线，见 §4）。
- 裁一次就判定超窗（估算与投影分叉时会误报；见 §10）。
- 靠程序僵硬地「钉死首条消息 / 首条 user 不被裁不压缩」：超长会话（数十 MB 级）的「首条
  user 消息」大概率是几周前的失效旧需求，强行保留既无效又架空模型自身的压缩判断。裁剪/压缩
  只需保 `system` + 最近 N 条（保留尾），早期历史随滑动窗口自然淘汰即可。
- 用列表下标（`msg-<n>`）当消息身份：`history_injection`、请求级裁剪、fork 都会让
  同一个引用指到另一条消息——那是「遮错内容」，不是「压得不准」。
- 在 agent 里做请求级裁剪、折叠、摘要化，或拿改写前的估算做「单轮超窗」硬报错
  （§7.1）。那既超出 agent 的职责（它只调度），也量的不是将要发出的那份表面。
- 宿主按插件名分发用量相关能力（应走声明式/事件）。

**守卫测试**（改坏了会红）：`plugin_names_guard_test.go`、`TestToolsFingerprint`、
`TestProjectPressure`、`TestLastUsageAnchor`、`TestRequestHeaderLogOnly`、
`TestEstimateProtoToolTokens`、`TestCompactionBasicMeasuresItsOwnSurface`、
`TestLsPipedIsOnePerLine`、`TestAgentSendsFullHistoryVerbatim`（agent 原样下发、
不裁不折）、`TestAgentDeclaresCompactionDependency`（压缩是强依赖）、
`TestStableIDsSurviveHeadTrimming`（id 不随列表起点变）、
`TestRetainBoundaryKeepsToolPairing`（压缩切点不切断 tool 配对）、
`TestFinishPreStepTrimsOversizedRequest`（超窗请求被收口压进窗口）、
`TestEstimateInflation`（偏差系数只放大不缩小、不因工具/窗口/模型变化作废、无锚点才退回 1）。

**构建期契约检查**：`tools/backendcheck`（builder 打包后自动跑，见 AGENTS.md
「共享模块改动的最小验证面」）。它按 `compaction` 能力扫全部插件，用真实派生历史驱动
**实际二进制**，校验结构约束（配对完整 / 条数不膨胀 / 首条 system 保留 / id 唯一且非
位置式）。这条通道专门抓「改了一处、另一处静默变形」——各插件自己的单测手搓消息，
天然绕过共享派生路径，抓不到。

---

## 10. 误差与实测基线

同一会话、同一份配置下的真实对照（未发额外请求，取自历史记录与本地函数计算）：

| 场景 | 我们估 | provider 实报 | 偏差 |
|---|---|---|---|
| 对话型内容（历史样本） | 318,562 | 332K | **−4%** |
| 压缩后稳态请求（37,790 含工具） | 37,790 | 42,098 | −10% |
| 全量历史（923,391 含工具） | 923,391 | 1,246,855 | −26% |

结论：

- **4 字节/token 对 JSON/代码/工具输出偏乐观**（那份请求约 3.18 字节/token），密集工具
  输出的会话误差可达 −26%；对话内容仍在 4% 量级。
- 有锚点时只用差量，这部分固定偏差会抵消；误差真正致命的场景是**无锚点的首次请求**。
- 若将来要收窄：优先做"按会话实测比例自适应"（用 `usage ÷ 该请求估算` 做修正），
  而不是改 divisor——后者会让纯对话内容变成高估。

---

## 11. 常见改动指南

**加一个估算维度**（如视频引用）：只需在 `EstimateProtoMessageTokens` 里加一项，
所有调用方自动生效；如需单独统计，再导出一个 `EstimateXxxTokens` 并让 SDK 转发。

**加一个会话事件**：见 §9「三处齐」；并在 `session/usage.go` 或相应投影里决定是否参与计量。

**换压缩后端**：它必须自己回答"我要发的表面是什么"，然后按 §7 决定读哪个数；
新后端不得直接照抄 `dsc-system` 的分支，而要先说清自己的表面是否被改写。

另外三条硬要求：

- 消息身份用 `proto.Message.id`（稳定 id），**不要**按列表下标编号（§7.1）。
- 状态文件带 `schema` 版本；格式变了就加一，旧文件整体丢弃、由事件日志重建，
  不写逐字段迁移（AGENTS.md §4）。
- 打包后跑 `builder`，`tools/backendcheck` 会按能力找到你并验结构约束；配对被切断、
  条数膨胀、首条 system 丢失、出现位置式 id 都会让构建失败。

**调阈值**：窗口 `DSC_CONTEXT_WINDOW`（宿主注入）/ `DSC_COMPACTION_BASIC_CONTEXT_WINDOW`（插件专属，优先）；
触发比例 `DSC_COMPACTION_BASIC_THRESHOLD`（默认 0.70：必须低于宿主 80% 收口，否则一次估算偏差就把它顶成整轮失败）；保留尾部 `DSC_COMPACTION_BASIC_RETAIN_RATIO`（默认 0.16）。
注意阈值比较的是"投影压力"，不是原始历史大小。

**动滑窗逻辑**（`core/context_ceiling.go` 的 `enforceRequestCeiling` / `CapMessagesToWindow`）：
这是**唯一**会丢历史的路径（agent 侧已无裁剪，§7.1）。三条不变量不能碰：

1. 保留段里的每条都是**原始消息**（同一对象、内容不变），只在开头插一行切除说明；
2. 切点必须是合法边界（首条 system 保留、不以 tool 结果开头、至少保留最后一条）；
3. 量的是**改写后**的表面（消息 + 工具目录），不是改写前。

改动时不得退回"裁一次就判超窗"——估算与投影分叉时会来回摆（「省略六千多条之后还报
441195 / 131072」就是这么来的）；也不得让提示帧回显裁剪前的占用（那会让人以为裁剪没
执行）。

---

## 12. 与 DSH 的对照

| DSH 做法 | DSC 现状 | 取舍理由 |
|---|---|---|
| `TokenMeter.measure(session, header)` 单一服务，用 session projection 重放 | 宿主 `projectPressure` + agent 本地 `TokenMeter` | 进程隔离：agent 必须本地决策 |
| `request/header` 独立事件 | 已引入，字段对齐（tools 指纹 + model + window） | 已对齐 |
| node 级表面差分（逐节点定价，替换只扣该段） | 整表估算 + 差量 | 结构不同（DSC 无 node 概念），精度略粗但判据一致 |
| usage 仅在 header 相同且不低于该表面启发式价时复用 | 同（§5.3、§6） | 已对齐 |
| `CHARS_PER_TOKEN=4` 固定密度 | CJK 感知分类累加 | DSH 自认该口径对 CJK/JSON 严重低估 |

---

## 13. 术语

| 术语 | 含义 |
|---|---|
| **表面（surface）** | 本次请求真正会被模型看到的内容 = 消息 + 工具目录（可能被压缩后端改写） |
| **锚点（anchor）** | 上一次成功请求的实测用量，附其信封指纹 |
| **信封（envelope）** | 产生该用量的请求身份：工具目录指纹 + 模型 + 窗口 |
| **投影（projection）** | 锚点 + 表面差量得到的"下一次大概多少"，用于决策 |
| **可比（comparable）** | 锚点信封与当前请求一致，且用量不低于该信封的启发式估值 |
| **稳定消息 id** | `proto.Message.id`，形如 `event-3921`（派生时取 surface 事件 seq）：唯一、与派生范围无关，是跨请求指认消息的唯一身份 |
