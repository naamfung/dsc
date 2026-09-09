package acp

import (
	"fmt"
	"sort"
	"strings"
)

// Decompress 恢复一个块的内容（对齐 acp-kernel decompress）。
// 默认恢复一层（T2→T1 摘要）；full=true 恢复到原始消息。
//
// MVP 实现：仅返回块的 summary 文本（T1 块无嵌套，无法恢复原始消息）。
// T2/T3 嵌套恢复留待后续实现。
func Decompress(blockID string, state *CompressionState, full bool) (string, error) {
	block := state.BlockByID(blockID)
	if block == nil {
		return "", fmt.Errorf("block %s not found", blockID)
	}
	if full {
		// MVP 限制：T1 块的原始消息已被压缩，无法恢复。
		// 后续可经 session 事件日志回放重建（event-sourced）。
		return block.Summary, fmt.Errorf("full decompress not implemented for T1 blocks (summary only)")
	}
	return block.Summary, nil
}

// SearchResults 搜索结果。
type SearchResult struct {
	BlockID    string `json:"blockId"`
	Topic      string `json:"topic"`
	Summary    string `json:"summary"`
	Score      int    `json:"score"` // 命中次数
	StartRef   string `json:"startRef"`
	EndRef     string `json:"endRef"`
}

// Search 在所有 active 块的 summary 中搜索关键词（子串匹配，对齐 acp-kernel substring 算法）。
// 后续可升级为 BM25 / fuzzy。
//
// 关键词按空格分割；命中次数越多分数越高。返回按 score 降序。
func Search(query string, state *CompressionState, limit int) []SearchResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	keywords := strings.Fields(strings.ToLower(query))
	if len(keywords) == 0 {
		return nil
	}

	var results []SearchResult
	for _, b := range state.Blocks {
		if !b.Active {
			continue
		}
		score := 0
		haystack := strings.ToLower(b.Summary + " " + b.Topic)
		for _, kw := range keywords {
			if strings.Contains(haystack, kw) {
				score++
			}
		}
		if score > 0 {
			results = append(results, SearchResult{
				BlockID:  b.BlockID,
				Topic:    b.Topic,
				Summary:  b.Summary,
				Score:    score,
				StartRef: b.StartRef,
				EndRef:   b.EndRef,
			})
		}
	}

	// 按 score 降序
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

// StatusReport 上下文状态报告（对齐 acp-kernel StatusReport）。
type StatusReport struct {
	ContextUsage     float64          `json:"contextUsage"`     // 0..1
	TokenCount       int              `json:"tokenCount"`
	ModelContextLimit int              `json:"modelContextLimit"`
	ActiveBlocks     int              `json:"activeBlocks"`
	TotalBlocks      int              `json:"totalBlocks"`
	TokensCompressed int              `json:"tokensCompressed"`
	CompressibleRanges []CompressibleRange `json:"compressibleRanges"`
	Breakdown        map[string]int   `json:"breakdown"`
}

// BuildStatus 构建状态报告（对齐 acp-kernel buildStatusReport）。
func BuildStatus(messages []CoreMessage, state *CompressionState, config Config, tokenCount int) StatusReport {
	usage := 0.0
	if config.ModelContextLimit > 0 {
		usage = float64(tokenCount) / float64(config.ModelContextLimit)
	}
	activeCount := 0
	for _, b := range state.Blocks {
		if b.Active {
			activeCount++
		}
	}
	return StatusReport{
		ContextUsage:       usage,
		TokenCount:         tokenCount,
		ModelContextLimit:  config.ModelContextLimit,
		ActiveBlocks:       activeCount,
		TotalBlocks:        len(state.Blocks),
		TokensCompressed:   state.Stats.TokensCompressed,
		CompressibleRanges: computeCompressibleRanges(messages, state, config),
		Breakdown: map[string]int{
			"tokensCompressed":   state.Stats.TokensCompressed,
			"compressionCount":   state.Stats.CompressionCount,
			"activeBlocks":       activeCount,
			"totalBlocks":        len(state.Blocks),
		},
	}
}

// FormatStatus 把状态报告格式化为模型可读文本。
func FormatStatus(report StatusReport) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ACP Context Status:\n"))
	sb.WriteString(fmt.Sprintf("  Usage: %d / %d tokens (%.0f%%)\n",
		report.TokenCount, report.ModelContextLimit, report.ContextUsage*100))
	sb.WriteString(fmt.Sprintf("  Blocks: %d active / %d total\n", report.ActiveBlocks, report.TotalBlocks))
	sb.WriteString(fmt.Sprintf("  Compressed: %d tokens reclaimed across %d compressions\n",
		report.TokensCompressed, report.Breakdown["compressionCount"]))
	if len(report.CompressibleRanges) > 0 {
		sb.WriteString(fmt.Sprintf("\nCompressible ranges (%d):\n", len(report.CompressibleRanges)))
		for i, r := range report.CompressibleRanges {
			sb.WriteString(fmt.Sprintf("  %d. %s..%s (%d messages, ~%d tokens)\n",
				i+1, r.StartRef, r.EndRef, r.Count, r.Tokens))
		}
	} else {
		sb.WriteString("\nNo compressible ranges (context is lean or all old content already compressed).\n")
	}
	return sb.String()
}

// ProcessTurnResult processTurn 的返回值（对齐 acp-kernel ProcessTurnResult）。
type ProcessTurnResult struct {
	Messages []CoreMessage     `json:"messages"`
	State    *CompressionState `json:"state"`
	Nudge    *NudgeDecision    `json:"nudge,omitempty"`
}

// ProcessTurn 每轮跑 pipeline（对齐 acp-kernel processTurn）。
// MVP pipeline：assign-refs → advance-survival → render-messages → decide-nudge
// （省略 sync-blocks merge / filter / hide-compress-calls / emergency-truncate）
//
// 输入：messages（本轮要发给 LLM 的消息列表）+ state + config + tokenCount
// 输出：改写后的 messages（含 <acp> 标签、应用 prune）+ 更新后的 state + nudge 决策
func ProcessTurn(messages []CoreMessage, state *CompressionState, config Config, tokenCount int) ProcessTurnResult {
	// 1. assign-refs：为新消息分配 mNNNNN
	AssignRefs(messages, state)

	// 2. advance-survival：推进块存活计数（驱动 young→old 升级）
	state.AdvanceSurvival(config.PromotionThreshold)

	// 3. render-messages：应用 prune + 注入 <acp> 标签
	rendered := RenderMessages(messages, state, config, true)

	// 4. decide-nudge：决策是否注入压缩提示
	nudge := DecideNudge(messages, state, config, tokenCount)

	return ProcessTurnResult{
		Messages: rendered,
		State:    state,
		Nudge:    &nudge,
	}
}
