package core

import (
        "context"
        "fmt"
        "sync"
        "time"
)

// 上下文压缩抽象缝（对齐 DSH/Cordis 的 ctx.compaction 能力缝）。
//
// DSH 的 compaction 是一个 Service 抽象（CompactionEngine），有三个方法：
//   - compactIfNeeded(agent, trigger, signal) → 压力驱动自动压缩
//   - compactNow(agent, signal, sourceCommandId) → 按需手动压缩
//   - compactRegion(start, end, agent, signal) → 强制压缩指定范围
//
// DSC 的适配：DSC 的 agent 是独立 gRPC 插件进程，压缩逻辑实现在 agent 内部。
// 宿主侧的压缩缝经 proto gRPC 接口暴露给 agent，使压缩策略可插拔——
// agent 调用宿主的压缩服务而非自行内联实现。
//
// 但当前 DSC 的压缩逻辑内联在 agent-react-loop 中（compactHistory 方法）。
// 本抽象缝先在宿主侧定义接口与默认实现，agent 可选择使用宿主服务或内联实现——
// 与 DSH 的 Service 抽象一致：实现可插拔，调用方不关心具体后端。

// CompactionTrigger 标识触发压缩的原因（对齐 DSH CompactionTrigger）。
type CompactionTrigger string

const (
        // CompactionTriggerPressure 正常压力驱动（上下文窗口使用率超过阈值）。
        CompactionTriggerPressure CompactionTrigger = "pressure"
        // CompactionTriggerOverflow 上下文溢出（provider 返回 context overflow 错误）。
        CompactionTriggerOverflow CompactionTrigger = "context-overflow"
)

// CompactionResult 一次成功压缩的结果（对齐 DSH CompactionResult）。
type CompactionResult struct {
        // CompactionId 稳定标识（时间戳生成，进程内唯一）。
        CompactionId string `json:"compaction_id"`
        // Summary 压缩生成的摘要内容。
        Summary string `json:"summary"`
        // ShadowedCount 被遮蔽（替换）的消息数。
        ShadowedCount int `json:"shadowed_count"`
        // ShadowedTokenEstimate 被遮蔽内容的估算 token 数。
        ShadowedTokenEstimate int `json:"shadowed_token_estimate"`
}

// CompactionEngine 压缩引擎接口（对齐 DSH CompactionEngine 抽象 Service）。
// 实现者拥有触发策略、保留策略与摘要生成逻辑。
type CompactionEngine interface {
        // CompactIfNeeded 检查是否需要压缩并在需要时执行。
        // trigger 标识触发原因（压力 / 溢出）。
        // 返回压缩结果；无需压缩时返回 nil。
        CompactIfNeeded(ctx context.Context, messages []*Message, trigger CompactionTrigger) (*CompactionResult, error)

        // CompactNow 按需执行压缩（即使未到阈值也强制压缩）。
        // 供 /compact 斜杆命令或模型主动触发使用。
        CompactNow(ctx context.Context, messages []*Message) (*CompactionResult, error)

        // CompactRegion 强制压缩指定范围 [start, end)（消息索引，左闭右开）。
        CompactRegion(ctx context.Context, messages []*Message, start, end int) (*CompactionResult, error)
}

// compactionConfig 压缩配置（对齐 DSH compaction-basic 的 Config）。
type compactionConfig struct {
        // ThresholdRatio 触发阈值比例（默认 0.80，即 80% 窗口使用率时触发）。
        ThresholdRatio float64
        // RetainRatio 保留尾部比例（默认 0.16，即保留最近 16% 窗口的消息不压缩）。
        RetainRatio float64
        // RetainTokensMin 保留尾部最少 token 数（默认 1024）。
        RetainTokensMin int
}

// defaultCompactionConfig 默认压缩配置（对齐 DSH compaction-basic 默认值）。
func defaultCompactionConfig() compactionConfig {
        return compactionConfig{
                ThresholdRatio:  0.80,
                RetainRatio:     0.16,
                RetainTokensMin: 1024,
        }
}

// BasicCompactionEngine 基础压缩引擎（对齐 DSH compaction-basic）。
// 使用 LLM 生成摘要，压力驱动 + 按需触发，保留尾部消息。
type BasicCompactionEngine struct {
        config        compactionConfig
        llmProvider   LLMProvider
        contextWindow int
        mu            sync.Mutex
        compacting    bool // 防重入：同一时间只允许一次压缩
}

// NewBasicCompactionEngine 创建基础压缩引擎。
// llmProvider 用于生成摘要；contextWindow 是上下文窗口大小（token 数）。
func NewBasicCompactionEngine(llmProvider LLMProvider, contextWindow int) *BasicCompactionEngine {
        return &BasicCompactionEngine{
                config:        defaultCompactionConfig(),
                llmProvider:   llmProvider,
                contextWindow: contextWindow,
        }
}

// estimateTokens 字节级启发式 token 估算（对齐 DSC 既有逻辑：约 4 字节/token）。
func estimateTokens(messages []*Message) int {
        total := 0
        for _, msg := range messages {
                total += len(msg.Content) / 4
        }
        return total
}

// CompactIfNeeded 检查上下文压力，超过阈值时自动压缩。
func (e *BasicCompactionEngine) CompactIfNeeded(ctx context.Context, messages []*Message, trigger CompactionTrigger) (*CompactionResult, error) {
        if len(messages) < 4 {
                return nil, nil // 消息太少，不值得压缩
        }

        estimatedTokens := estimateTokens(messages)
        threshold := int(float64(e.contextWindow) * e.config.ThresholdRatio)

        // 压力触发：仅在超过阈值时压缩
        if trigger == CompactionTriggerPressure && estimatedTokens < threshold {
                return nil, nil
        }

        // 溢出触发：即使未到阈值也强制压缩（provider 已返回 overflow 错误）
        return e.doCompact(ctx, messages, false)
}

// CompactNow 按需压缩（强制，不检查阈值）。
func (e *BasicCompactionEngine) CompactNow(ctx context.Context, messages []*Message) (*CompactionResult, error) {
        if len(messages) < 2 {
                return nil, nil
        }
        return e.doCompact(ctx, messages, true)
}

// CompactRegion 强制压缩指定范围 [start, end)。
func (e *BasicCompactionEngine) CompactRegion(ctx context.Context, messages []*Message, start, end int) (*CompactionResult, error) {
        if start < 0 || end > len(messages) || start >= end {
                return nil, fmt.Errorf("invalid region [%d, %d) for %d messages", start, end, len(messages))
        }
        return e.doCompactRegion(ctx, messages, start, end)
}

// doCompact 执行压缩：选择可压缩范围（保留尾部），生成摘要，返回结果。
// force=true 时即使所有消息都落入保留区也强制压缩前半段（CompactNow 用）。
func (e *BasicCompactionEngine) doCompact(ctx context.Context, messages []*Message, force bool) (*CompactionResult, error) {
        e.mu.Lock()
        if e.compacting {
                e.mu.Unlock()
                return nil, fmt.Errorf("compaction already in progress")
        }
        e.compacting = true
        e.mu.Unlock()

        defer func() {
                e.mu.Lock()
                e.compacting = false
                e.mu.Unlock()
        }()

        // 计算保留尾部：从末尾向前累积，保留 RetainRatio 比例或 RetainTokensMin（取大者）
        retainTokens := int(float64(e.contextWindow) * e.config.RetainRatio)
        if retainTokens < e.config.RetainTokensMin {
                retainTokens = e.config.RetainTokensMin
        }

        retainIdx := len(messages)
        acc := 0
        for i := len(messages) - 1; i >= 0; i-- {
                acc += len(messages[i].Content) / 4
                if acc > retainTokens {
                        retainIdx = i + 1
                        break
                }
        }

        // 循环未触发 break：所有消息总 token 都在保留区内。
        //   - force=false（CompactIfNeeded 路径）：消息本就很小，无需压缩（return nil）
        //   - force=true（CompactNow 路径）：仍需压缩，保留最后 1 条，压缩前 n-1 条
        if retainIdx >= len(messages) {
                if !force {
                        return nil, nil
                }
                // CompactNow 强制压缩：保留最后 1 条，压缩前面所有
                retainIdx = len(messages) - 1
                if retainIdx < 1 {
                        return nil, nil // 只有 1 条消息没法压缩
                }
        }

        // 没有可压缩的范围（保留的几乎就是全部）
        if retainIdx <= 1 {
                // 仅 1 条消息不值得压缩；强制模式下也无能为力
                if force && len(messages) >= 2 {
                        retainIdx = 1 // 压缩首条
                } else {
                        return nil, nil
                }
        }

        return e.doCompactRegion(ctx, messages, 0, retainIdx)
}

// doCompactRegion 压缩指定范围 [start, end) 的消息为一条摘要。
func (e *BasicCompactionEngine) doCompactRegion(ctx context.Context, messages []*Message, start, end int) (*CompactionResult, error) {
        if e.llmProvider == nil {
                // 无 LLM provider（如 agent 尚未激活时）：退化为截断式压缩（取首条+末条拼接）
                return e.truncateCompact(messages, start, end), nil
        }

        // 构建压缩请求：把待压缩的消息拼成 system prompt + user 消息
        compactPrompt := "你是对话压缩器。请将下面的对话历史压缩成一段精简但信息完整的摘要，" +
                "保留关键信息（用户意图、重要决策、工具结果要点），去除冗余细节。只输出摘要，不添加额外解释。\n\n--- 对话历史 ---\n"

        for i := start; i < end; i++ {
                msg := messages[i]
                compactPrompt += fmt.Sprintf("[%s] %s\n", msg.Role, msg.Content)
        }

        compactMessages := []Message{
                {Role: "system", Content: compactPrompt},
                {Role: "user", Content: "请压缩上述对话历史。"},
        }

        // 调用 LLM 生成摘要（不带工具，maxTokens 限制为窗口的 1/8）
        maxTokens := e.contextWindow / 8
        if maxTokens < 512 {
                maxTokens = 512
        }
        resp, err := e.llmProvider.Chat(ctx, compactMessages, nil, maxTokens)
        if err != nil {
                return nil, fmt.Errorf("compaction LLM call failed: %w", err)
        }

        if resp.Content == "" {
                return nil, fmt.Errorf("compaction returned empty summary")
        }

        shadowedTokens := 0
        for i := start; i < end; i++ {
                shadowedTokens += len(messages[i].Content) / 4
        }

        return &CompactionResult{
                CompactionId:          fmt.Sprintf("compact-%d", time.Now().UnixMilli()),
                Summary:               resp.Content,
                ShadowedCount:         end - start,
                ShadowedTokenEstimate: shadowedTokens,
        }, nil
}

// truncateCompact 退化压缩（无 LLM 时）：取首条+末条消息拼接为摘要。
func (e *BasicCompactionEngine) truncateCompact(messages []*Message, start, end int) *CompactionResult {
        var summary string
        if end-start > 0 {
                summary = "[压缩摘要] " + messages[start].Content
                if end-start > 1 {
                        summary += "\n...\n" + messages[end-1].Content
                }
        }
        shadowedTokens := 0
        for i := start; i < end; i++ {
                shadowedTokens += len(messages[i].Content) / 4
        }
        return &CompactionResult{
                CompactionId:          fmt.Sprintf("compact-%d", time.Now().UnixMilli()),
                Summary:               summary,
                ShadowedCount:         end - start,
                ShadowedTokenEstimate: shadowedTokens,
        }
}

// SetCompactionEngine 注入压缩引擎后端（对齐 DSH ctx.compaction = CompactionEngine）。
// 供 billion-context 等插件在加载时调用，替换默认的内联 compactHistory 路径。
// 传 nil 恢复默认（无后端，agent 走内联压缩）。
//
// DSC 的压缩后端接管机制与 DSH 不同：DSH 经 Cordis Service 单例（ctx.compaction）
// 让 agent-loop 调用；DSC 经 agent/pre-step 事件让插件改写消息列表。两者效果等价
// （后端在更低阈值主动压缩，agent 内联压缩不被触发），但机制不同。本方法保留
// CompactionEngine 接口供未来可能的 Service 注入路径使用，当前接管走事件机制。
func (m *Manager) SetCompactionEngine(engine CompactionEngine) {
        m.mu.Lock()
        defer m.mu.Unlock()
        m.compaction = engine
}

// HasCompactionEngine 报告是否有压缩引擎后端已注入。
// 当前接管机制走 agent/pre-step 事件（DSC_COMPACTION_BACKEND 环境变量），
// 此方法供宿主内部查询是否有 Service 注入路径的后端（预留扩展）。
func (m *Manager) HasCompactionEngine() bool {
        m.mu.RLock()
        defer m.mu.RUnlock()
        return m.compaction != nil
}
