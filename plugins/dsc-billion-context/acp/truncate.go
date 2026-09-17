package acp

import (
	"fmt"
	"sort"
	"strings"
)

// emergency-truncate 节点（对齐 acp-kernel truncate.ts + truncate-tools.ts）：
// pipeline 最后的 token 回收安全阀——上下文占用达到 TruncateThreshold 时，
// 把最大的工具结果（以及 #300 引入的最后手段文本消息）截断为
// 「头前缀 + 截断标记 + 尾后缀」，保留最近 PreserveRecentMessages 条不碰。
//
// UTF-8 安全截断：Go string 按字节索引，直接切片可能切断多字节字符
// （对齐上游 #816 lone-surrogate 中毒问题的 UTF-16 场景）——此处一律按
// rune 边界切分，clampPrefix/clampWindow 保证不产生残缺字符。

// truncationMarker 截断标记（模型可见，二次截断跳过已标记消息）。
const truncationMarker = "[truncated for context space]"

// truncateDefaults 紧急截断默认参数（对齐 truncate-tools.ts DEFAULTS）。
const (
	truncateMinOutputTokens  = 1000
	truncateKeepPrefixChars  = 2000
	truncateKeepSuffixChars  = 2000
	truncateProtectRecent    = 3
	truncateTargetShrinkPct  = 0.9 // 目标降到阈值线以下的 90%，避免贴线反复触发
	minTokensForLastResortTx = 0
)

// clampPrefix 取至多 maxRunes 个 rune 的前缀（rune 边界安全）。
func clampPrefix(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

// clampWindow 取 [startRunes, endRunes) 窗口（rune 边界安全，越界收敛）。
func clampWindow(text string, startRunes, endRunes int) string {
	runes := []rune(text)
	if startRunes < 0 {
		startRunes = 0
	}
	if endRunes > len(runes) {
		endRunes = len(runes)
	}
	if startRunes > endRunes {
		startRunes = endRunes
	}
	return string(runes[startRunes:endRunes])
}

// runeLen 文本的 rune 数。
func runeLen(text string) int { return len([]rune(text)) }

// TruncateResult 紧急截断结果。
type TruncateResult struct {
	Messages        []CoreMessage `json:"messages"`
	TruncatedCount  int           `json:"truncatedCount"`
	SavedTokens     int           `json:"savedTokens"`
	CandidatesFound int           `json:"candidatesFound"` // 达标候选数（0 解释阈值上静默无操作）
}

// TruncateLargeToolOutputs 紧急截断超大工具输出（对齐 acp-kernel truncateLargeToolOutputs）。
//
// 两阶段候选（对齐 #300）：
//  1. tool-result（主候选，按 token 数降序）
//  2. user/assistant 文本消息（includeTextMessages=true 时，最后手段）——
//     渲染 summary 永不触碰（它们是压缩内容的唯一持久记录）
//
// 近端保护：最后 PreserveRecentMessages 条不参与。
func TruncateLargeToolOutputs(messages []CoreMessage, tokenCount int, config Config, includeTextMessages bool) TruncateResult {
	limit := config.ModelContextLimit
	if limit <= 0 || float64(tokenCount) < config.TruncateThreshold*float64(limit) {
		return TruncateResult{Messages: messages}
	}

	protectRecent := config.PreserveRecentMessages
	if protectRecent <= 0 {
		protectRecent = truncateProtectRecent
	}
	protectedIndex := len(messages) - protectRecent

	findCandidates := func(match func(CoreMessage) bool) []int {
		var found []int
		for i := 0; i < protectedIndex && i < len(messages); i++ {
			m := messages[i]
			if !match(m) {
				continue
			}
			if m.Text == "" || strings.Contains(m.Text, truncationMarker) {
				continue
			}
			if estimateTokensForText(m.Text) < truncateMinOutputTokens {
				continue
			}
			found = append(found, i)
		}
		// 按 token 数降序（最大的先截，收益最大）
		sort.Slice(found, func(a, b int) bool {
			return estimateTokensForText(messages[found[a]].Text) > estimateTokensForText(messages[found[b]].Text)
		})
		return found
	}

	isRenderedSummaryMsg := func(m CoreMessage) bool {
		return isRenderedSummary(m) || strings.HasPrefix(m.Text, "[Compressed conversation section]")
	}

	toolResults := findCandidates(func(m CoreMessage) bool { return m.ContentType == ContentTypeToolResult })
	var textMessages []int
	if includeTextMessages {
		textMessages = findCandidates(func(m CoreMessage) bool {
			return m.ContentType == ContentTypeText &&
				(m.Role == RoleUser || m.Role == RoleAssistant) &&
				!isRenderedSummaryMsg(m)
		})
	}
	candidatesFound := len(toolResults) + len(textMessages)

	truncatedCount := 0
	savedTokens := 0
	remaining := tokenCount
	targetTokens := int(config.TruncateThreshold * float64(limit) * truncateTargetShrinkPct)
	replacement := map[int]string{}

	applyTo := func(candidates []int) {
		for _, i := range candidates {
			if remaining <= targetTokens {
				break
			}
			original := messages[i].Text
			tokens := estimateTokensForText(original)
			if runeLen(original) <= truncateKeepPrefixChars+truncateKeepSuffixChars {
				continue
			}
			prefix := clampPrefix(original, truncateKeepPrefixChars)
			suffix := clampWindow(original, runeLen(original)-truncateKeepSuffixChars, runeLen(original))
			replacement[i] = fmt.Sprintf("%s\n\n...%s — original ~%d tokens]...\n\n%s",
				prefix, truncationMarker, tokens, suffix)
			truncatedCount++
			saved := tokens - estimateTokensForText(replacement[i])
			remaining -= saved
			savedTokens += saved
		}
	}

	applyTo(toolResults)
	applyTo(textMessages)

	if len(replacement) == 0 {
		return TruncateResult{Messages: messages, CandidatesFound: candidatesFound}
	}
	out := make([]CoreMessage, len(messages))
	for i := range messages {
		if txt, ok := replacement[i]; ok {
			out[i] = messages[i]
			out[i].Text = txt
			continue
		}
		out[i] = messages[i]
	}
	return TruncateResult{Messages: out, TruncatedCount: truncatedCount, SavedTokens: savedTokens, CandidatesFound: candidatesFound}
}
