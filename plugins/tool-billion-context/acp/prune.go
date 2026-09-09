package acp

import "fmt"

// PruneRange 一个待压缩的范围（对齐 acp-kernel CompressRangeSpec）。
type PruneRange struct {
	StartRef string `json:"startRef"` // 范围起始 ref（mNNNNN）
	EndRef   string `json:"endRef"`   // 范围结束 ref（mNNNNN，含）
	Summary  string `json:"summary"`  // 模型生成的摘要
	Topic    string `json:"topic,omitempty"` // 可选主题
}

// ApplyCompression 应用一次压缩：分配块、遮蔽被压缩范围、更新统计。
// 对齐 acp-kernel applyCompression。
//
// 输入：ranges（模型调用 compress 工具时提供）+ messages + state + config
// 输出：更新后的 state + result（blocksCreated / tokensCompressed / errors）
//
// 压缩协议：
//  1. 解析每个 range 的 startRef/endRef 为消息索引
//  2. 收集范围内的消息 ID（directMessageIDs）+ 受保护消息排除
//  3. 分配 block ID，记录 summary、topic、tier=1、active=true
//  4. 更新统计（tokensCompressed += 被压缩消息的估算 token 数）
//  5. 不修改 messages 列表本身——prune 的实际替换由 pipeline 的 prune 节点完成
//     （state 只记录"哪些消息被覆盖"，渲染时由 RenderMessages 应用遮蔽）
func ApplyCompression(ranges []PruneRange, messages []CoreMessage, state *CompressionState, config Config) (*CompressionState, CompressionResult) {
	result := CompressionResult{}
	runID := state.AllocateRunID()
	covered := state.CoveredMessageIDs()

	for _, r := range ranges {
		startIdx, endIdx, ok := resolveRange(r.StartRef, r.EndRef, messages, state)
		if !ok {
			result.Errors = append(result.Errors,
				fmt.Sprintf("range %s..%s: cannot resolve boundaries", r.StartRef, r.EndRef))
			continue
		}
		if startIdx > endIdx {
			result.Errors = append(result.Errors,
				fmt.Sprintf("range %s..%s: start after end", r.StartRef, r.EndRef))
			continue
		}
		// 收集范围内的消息 ID（排除已覆盖的——避免重复压缩）
		var directIDs []string
		var effectiveIDs []string
		var compressedTokens int
		for i := startIdx; i <= endIdx; i++ {
			msg := messages[i]
			if msg.ID == "" {
				continue
			}
			if covered[msg.ID] {
				continue // 已被其他块覆盖
			}
			directIDs = append(directIDs, msg.ID)
			effectiveIDs = append(effectiveIDs, msg.ID)
			compressedTokens += estimateTokensForText(msg.Text)
		}
		if len(directIDs) == 0 {
			result.Errors = append(result.Errors,
				fmt.Sprintf("range %s..%s: no compressible messages (all already covered or empty)", r.StartRef, r.EndRef))
			continue
		}
		// 分配块
		block := CompressionBlock{
			BlockID:            state.AllocateBlockID(),
			RunID:             runID,
			Tier:              Tier1,
			Topic:             r.Topic,
			Summary:           r.Summary,
			DirectMessageIDs:  directIDs,
			EffectiveMessageIDs: effectiveIDs,
			CompressedTokens:   compressedTokens,
			CreatedAt:          nowMillis(),
			SurvivedCount:      0,
			Generation:         GenYoung,
			Active:             true,
			StartRef:           r.StartRef,
			EndRef:             r.EndRef,
		}
		state.Blocks = append(state.Blocks, block)
		result.BlocksCreated++
		result.TokensCompressed += compressedTokens
		state.Stats.TokensCompressed += compressedTokens
		state.Stats.CompressionCount++
	}

	// 压缩成功后重置 nudge baseline（防反馈循环：避免压缩后立即重新 nudge）
	state.Nudge.BaselineTokens = 0
	return state, result
}

// CompressionResult 一次压缩操作的结果。
type CompressionResult struct {
	BlocksCreated    int      `json:"blocksCreated"`
	TokensCompressed int      `json:"tokensCompressed"`
	Errors           []string `json:"errors,omitempty"`
}

// resolveRange 把 startRef/endRef 解析为消息索引 [start, end]（含）。
// ref 不存在或不在消息列表中时返回 ok=false。
func resolveRange(startRef, endRef string, messages []CoreMessage, state *CompressionState) (startIdx, endIdx int, ok bool) {
	startID := RawForRef(startRef, state)
	endID := RawForRef(endRef, state)
	if startID == "" || endID == "" {
		return 0, 0, false
	}
	startIdx, endIdx = -1, -1
	for i, msg := range messages {
		if msg.ID == startID {
			startIdx = i
		}
		if msg.ID == endID {
			endIdx = i
		}
	}
	if startIdx < 0 || endIdx < 0 {
		return 0, 0, false
	}
	return startIdx, endIdx, true
}

// RenderMessages 把消息列表渲染为模型可见的形态：
//  - 被压缩块覆盖的消息替换为 summary（保留 ref 标签）
//  - 保留尾部 N 条消息不压缩（preserveRecent）
//  - 注入 <acp tokens="N">mNNNNN</acp> 标签到每条消息（可选，由 renderTags 控制）
//
// 对齐 acp-kernel prune + render-refs 节点。
func RenderMessages(messages []CoreMessage, state *CompressionState, config Config, renderTags bool) []CoreMessage {
	if len(messages) == 0 {
		return messages
	}
	covered := state.CoveredMessageIDs()
	out := make([]CoreMessage, 0, len(messages)+len(state.Blocks))

	// 先输出所有 active 块的 summary（按创建时间顺序，作为 assistant 消息）
	activeBlocks := state.ActiveBlocks()
	for _, b := range activeBlocks {
		summaryText := formatBlockSummary(b)
		out = append(out, CoreMessage{
			ID:          "block:" + b.BlockID,
			Role:        RoleAssistant,
			ContentType: ContentTypeText,
			Text:        summaryText,
		})
	}

	// 输出未被覆盖的消息（尾部 preserveRecent 条永远保留）
	preserveFromIdx := len(messages) - config.PreserveRecent
	if preserveFromIdx < 0 {
		preserveFromIdx = 0
	}
	for i, msg := range messages {
		if msg.ID == "" {
			// 块 summary 等内部消息直接保留
			out = append(out, msg)
			continue
		}
		if covered[msg.ID] && i >= preserveFromIdx {
			// 在保留区内即使被覆盖也保留（避免丢失最近上下文）
			out = append(out, renderTaggedMessage(msg, state, renderTags))
			continue
		}
		if covered[msg.ID] {
			// 已被压缩且不在保留区：跳过（summary 已在前面输出）
			continue
		}
		out = append(out, renderTaggedMessage(msg, state, renderTags))
	}
	return out
}

// renderTaggedMessage 给消息注入 <acp> 标签（如果 renderTags 为 true 且消息有 ref）。
func renderTaggedMessage(msg CoreMessage, state *CompressionState, renderTags bool) CoreMessage {
	if !renderTags {
		return msg
	}
	ref := RefForRaw(msg.ID, state)
	if ref == "" {
		return msg
	}
	tokens := estimateTokensForText(msg.Text)
	tag := fmt.Sprintf(`<acp tokens="%d" type="%s">%s</acp>`, tokens, msg.ContentType, ref)
	if msg.Text == "" {
		msg.Text = tag
	} else {
		msg.Text = msg.Text + "\n" + tag
	}
	return msg
}

// formatBlockSummary 格式化块的 summary 为模型可见文本。
func formatBlockSummary(b CompressionBlock) string {
	topic := b.Topic
	if topic == "" {
		topic = "compressed range"
	}
	return fmt.Sprintf("[ACP Block %s: %s (%s..%s, %d messages, ~%d tokens)]\n%s",
		b.BlockID, topic, b.StartRef, b.EndRef,
		len(b.DirectMessageIDs), b.CompressedTokens, b.Summary)
}

// estimateTokensForText 字节/CJK 启发式 token 估算（对齐 acp-kernel estimateTokensFast）。
// 取「字节数/4」与「rune 数」的较大者——英文按 /4（约 4 字符 1 token）；
// CJK（UTF-8 每字 3 字节）字节/4 会低估，故回调为 rune 数（每字按 1 token）。
func estimateTokensForText(s string) int {
	if s == "" {
		return 0
	}
	bytes := len(s)
	runes := 0
	for range s {
		runes++
	}
	byBytes := (bytes + 3) / 4
	if byBytes > runes {
		return byBytes
	}
	return runes
}

// nowMillis 当前时间戳（毫秒）。
func nowMillis() int64 {
	// 避免在核心包引入 time 依赖（保持纯函数特性）；
	// adapter 层可注入自定义时钟。此处用 time.Now() 是务实选择。
	return timeNowMillis()
}
