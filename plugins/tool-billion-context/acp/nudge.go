package acp

import (
        "fmt"
        "sort"
        "strings"
)

// NudgeDecision nudge 决策结果（对齐 acp-kernel NudgeDecision）。
type NudgeDecision struct {
        ShouldInject       bool               `json:"shouldInject"`
        Reason             string             `json:"reason"`
        CompressibleRanges []CompressibleRange `json:"compressibleRanges"`
        ContextUsage       float64            `json:"contextUsage"` // 0..1
        Breakdown          map[string]int     `json:"breakdown"`
        // Tier 触发的蒸馏层级（对齐 acp-kernel NudgeDecision.tier）：
        //   nil  — 不触发蒸馏（仅标准 T1 nudge 或无 nudge）
        //   2    — T1 块数达 Tier2Trigger，提示蒸馏为 T2
        //   3    — T2 块数达 Tier3Trigger，提示凝结为 T3
        Tier *CompressionTier `json:"tier,omitempty"`
        // TierTargetBlocks 待蒸馏的目标块列表（对齐 acp-kernel tierTargetBlocks）。
        // 模型在 compress 调用中用这些块的 bN 作为 startId/endId。
        TierTargetBlocks []CompressionBlock `json:"tierTargetBlocks,omitempty"`
}

// CompressibleRange 一个可压缩范围（供模型在 compress 调用中引用）。
type CompressibleRange struct {
        StartRef string `json:"startRef"`
        EndRef   string `json:"endRef"`
        Count    int    `json:"count"`
        Tokens   int    `json:"tokens"`
}

// DecideNudge 决策是否注入 nudge 提示（对齐 acp-kernel decideNudge）。
//
// 触发条件：
//  1. 上下文使用率 ≥ NudgeThresholdPct（如 45%）
//  2. 自上次 nudge 后有增长（防反馈循环）
//  3. 存在可压缩范围（被覆盖消息外、保留区外）
//
// 增长门控（growth-gating）：
//  - baselineTokens 在压缩成功后被重置为 0（见 ApplyCompression）
//  - 下次 nudge 要求 currentTokens > baselineTokens + growthFloor
//  - 这防止"压缩后立即又触发 nudge"的反馈循环
func DecideNudge(messages []CoreMessage, state *CompressionState, config Config, tokenCount int) NudgeDecision {
        decision := NudgeDecision{Breakdown: map[string]int{}}

        // 计算上下文使用率
        usage := 0.0
        if config.ModelContextLimit > 0 {
                usage = float64(tokenCount) / float64(config.ModelContextLimit)
        }
        decision.ContextUsage = usage
        decision.Breakdown["usage"] = tokenCount
        decision.Breakdown["modelContextLimit"] = config.ModelContextLimit

        // 阈值门控
        thresholdTokens := int(float64(config.ModelContextLimit) * config.NudgeThresholdPct)
        decision.Breakdown["thresholdTokens"] = thresholdTokens
        if tokenCount < thresholdTokens {
                decision.Reason = fmt.Sprintf("below threshold (%d < %d)", tokenCount, thresholdTokens)
                return decision
        }

        // 增长门控：自上次 nudge/baseline 后必须有增长
        growthFloor := 1000 // 至少增长 1000 token 才重新 nudge
        if state.Nudge.BaselineTokens > 0 {
                if tokenCount-state.Nudge.BaselineTokens < growthFloor {
                        decision.Reason = fmt.Sprintf("insufficient growth since baseline (%d - %d < %d)",
                                tokenCount, state.Nudge.BaselineTokens, growthFloor)
                        return decision
                }
        }
        decision.Breakdown["growthFloor"] = growthFloor
        decision.Breakdown["baselineTokens"] = state.Nudge.BaselineTokens

        // 计算可压缩范围
        ranges := computeCompressibleRanges(messages, state, config)
        decision.CompressibleRanges = ranges
        decision.Breakdown["compressibleRanges"] = len(ranges)
        if len(ranges) == 0 {
                decision.Reason = "no compressible ranges"
                return decision
        }

        // 检查 T2/T3 蒸馏触发条件（对齐 acp-kernel tier distillation）
        // T2：active T1 块数达 Tier2Trigger
        // T3：active T2 块数达 Tier3Trigger
        activeBlocks := state.ActiveBlocks()
        t1Count, t2Count := 0, 0
        var t1Blocks, t2Blocks []CompressionBlock
        for _, b := range activeBlocks {
                if b.Tier == Tier1 {
                        t1Count++
                        t1Blocks = append(t1Blocks, b)
                } else if b.Tier == Tier2 {
                        t2Count++
                        t2Blocks = append(t2Blocks, b)
                }
        }
        decision.Breakdown["t1Count"] = t1Count
        decision.Breakdown["t2Count"] = t2Count

        // T3 蒸馏优先（更高层级，更经济）：T2 块数达 Tier3Trigger
        if config.Tiers.Tier3Trigger > 0 && t2Count >= config.Tiers.Tier3Trigger {
                tier := Tier3
                decision.Tier = &tier
                decision.TierTargetBlocks = t2Blocks
                decision.ShouldInject = true
                decision.Reason = fmt.Sprintf("T3 condense: %d tier-2 blocks >= tier3Trigger %d, usage %.0f%%",
                        t2Count, config.Tiers.Tier3Trigger, usage*100)
                return decision
        }

        // T2 蒸馏：T1 块数达 Tier2Trigger
        if config.Tiers.Tier2Trigger > 0 && t1Count >= config.Tiers.Tier2Trigger {
                tier := Tier2
                decision.Tier = &tier
                decision.TierTargetBlocks = t1Blocks
                decision.ShouldInject = true
                decision.Reason = fmt.Sprintf("T2 distill: %d tier-1 blocks >= tier2Trigger %d, usage %.0f%%",
                        t1Count, config.Tiers.Tier2Trigger, usage*100)
                return decision
        }

        // 全部条件满足：注入 nudge
        decision.ShouldInject = true
        decision.Reason = fmt.Sprintf("context usage %.0f%% (≥%.0f%% threshold), %d compressible ranges",
                usage*100, config.NudgeThresholdPct*100, len(ranges))
        return decision
}

// computeCompressibleRanges 计算可压缩范围（对齐 acp-kernel buildCompressibleRanges）。
// 跳过：已被覆盖的消息、保留区内的尾部消息、受保护工具的 tool-call/result。
func computeCompressibleRanges(messages []CoreMessage, state *CompressionState, config Config) []CompressibleRange {
        covered := state.CoveredMessageIDs()
        protected := map[string]bool{}
        for _, t := range config.ProtectedTools {
                protected[t] = true
        }

        preserveFromIdx := len(messages) - config.PreserveRecent
        if preserveFromIdx < 0 {
                preserveFromIdx = 0
        }

        // 收集可压缩消息的索引（连续段合并为 range）
        var ranges []CompressibleRange
        var curStart, curEnd, curTokens int
        inRange := false

        for i, msg := range messages {
                // 跳过保留区
                if i >= preserveFromIdx {
                        break
                }
                // 跳过已覆盖
                if msg.ID != "" && covered[msg.ID] {
                        if inRange {
                                ranges = append(ranges, finalizeRange(messages, curStart, curEnd, curTokens, state))
                                inRange = false
                        }
                        continue
                }
                // 跳过受保护工具
                if msg.ToolName != "" && protected[msg.ToolName] {
                        if inRange {
                                ranges = append(ranges, finalizeRange(messages, curStart, curEnd, curTokens, state))
                                inRange = false
                        }
                        continue
                }
                // 跳过块 summary（ID 形如 block:b0）
                if strings.HasPrefix(msg.ID, "block:") {
                        if inRange {
                                ranges = append(ranges, finalizeRange(messages, curStart, curEnd, curTokens, state))
                                inRange = false
                        }
                        continue
                }
                // 进入或扩展 range
                if !inRange {
                        curStart = i
                        curTokens = 0
                        inRange = true
                }
                curEnd = i
                curTokens += estimateTokensForText(msg.Text)
        }
        if inRange {
                ranges = append(ranges, finalizeRange(messages, curStart, curEnd, curTokens, state))
        }

        // 合并过小的 range（< 500 token 不值得压缩）
        filtered := ranges[:0]
        for _, r := range ranges {
                if r.Tokens >= 500 {
                        filtered = append(filtered, r)
                }
        }
        return filtered
}

// finalizeRange 把 [startIdx, endIdx] 范围转为 CompressibleRange。
func finalizeRange(messages []CoreMessage, startIdx, endIdx, tokens int, state *CompressionState) CompressibleRange {
        startRef := ""
        endRef := ""
        count := 0
        for i := startIdx; i <= endIdx; i++ {
                if messages[i].ID == "" {
                        continue
                }
                ref := RefForRaw(messages[i].ID, state)
                if ref == "" {
                        continue
                }
                if startRef == "" {
                        startRef = ref
                }
                endRef = ref
                count++
        }
        return CompressibleRange{
                StartRef: startRef,
                EndRef:   endRef,
                Count:    count,
                Tokens:   tokens,
        }
}

// FormatNudgeText 把 nudge 决策渲染为模型可见的提示文本。
// 对齐 acp-kernel renderNudgeText。
func FormatNudgeText(d NudgeDecision) string {
        if !d.ShouldInject {
                return ""
        }
        var sb strings.Builder
        sb.WriteString(fmt.Sprintf("[ACP Nudge: %s]\n", d.Reason))

        // 蒸馏提示（T2/T3）：列出待蒸馏的目标块
        if d.Tier != nil && len(d.TierTargetBlocks) > 0 {
                tier := *d.Tier
                if tier == Tier2 {
                        sb.WriteString(fmt.Sprintf("Your tier-1 compression summaries have accumulated (%d blocks). ",
                                len(d.TierTargetBlocks)))
                        sb.WriteString("Distill them into a single denser tier-2 summary. ")
                        sb.WriteString("Use block IDs as boundaries (startId and endId as bN).\n")
                } else if tier == Tier3 {
                        sb.WriteString(fmt.Sprintf("Your tier-2 compression summaries have accumulated (%d blocks). ",
                                len(d.TierTargetBlocks)))
                        sb.WriteString("Condense them further into a tier-3 ultra-condensed summary. ")
                        sb.WriteString("Use block IDs as boundaries (startId and endId as bN).\n")
                }
                // 列出目标块
                sb.WriteString("Target blocks to distill:\n")
                for i, b := range d.TierTargetBlocks {
                        topic := b.Topic
                        if topic == "" {
                                topic = "untitled"
                        }
                        sb.WriteString(fmt.Sprintf("  %d. %s (tier-%d, %s, ~%d tokens)\n",
                                i+1, b.BlockID, b.Tier, topic, b.CompressedTokens))
                }
                sb.WriteString("\nExample: compress({content:[{startId:\"b0\", endId:\"b4\", summary:\"...\", topic:\"...\"}]})\n")
                return sb.String()
        }

        // 标准 T1 压缩提示
        if len(d.CompressibleRanges) > 0 {
                sb.WriteString("Compressible ranges (call `compress` to reclaim context):\n")
                // 按 tokens 降序，让模型优先压缩最大的范围
                ranges := make([]CompressibleRange, len(d.CompressibleRanges))
                copy(ranges, d.CompressibleRanges)
                sort.Slice(ranges, func(i, j int) bool { return ranges[i].Tokens > ranges[j].Tokens })
                for i, r := range ranges {
                        sb.WriteString(fmt.Sprintf("  %d. %s..%s (%d messages, ~%d tokens)\n",
                                i+1, r.StartRef, r.EndRef, r.Count, r.Tokens))
                }
                sb.WriteString("\nUse the `compress` tool with content=[{startId, endId, summary, topic?}] to compress a range.")
        }
        return sb.String()
}
