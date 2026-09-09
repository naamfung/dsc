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

// RenderMessages 把消息列表渲染为模型可见的形态（对齐 acp-kernel prune + render-refs）。
//
// 前缀稳定设计（核心语义，对齐原作者描述）：
//
//      [summary1 (~2K)] [summary2 (~2K)] ... [最近 5万 token 新生成]
//
//  - 每个 summary 放在它替换的原始范围的最早消息位置（insertAt），不是堆到列表开头
//  - 一旦 summary 写定，后续轮次渲染时经 summaryMessageId 识别"已渲染的 summary"，
//    保持其位置稳定——前缀缓存命中率因此达 98-99%
//  - 只有触发二级蒸馏（T1 summary 累计过多→T2）时前缀才会失效
//
// 角色用 system 而非 assistant：system 消息稳定，provider 不会因角色变化重排缓存。
//
// 缺少 orphaned tool-call/result 配对清理的 MVP 限制：压缩边界若切断
// tool-call ↔ tool-result 配对，某些 provider 会报错。原版 acp-kernel 经
// stripOrphanedToolResults / stripOrphanedToolCalls 清理——MVP 暂未实现，
// 依赖模型压缩时选择"完整工具回合"作为范围（推荐在 system prompt 中引导）。
func RenderMessages(messages []CoreMessage, state *CompressionState, config Config, renderTags bool) []CoreMessage {
        if len(messages) == 0 {
                return messages
        }
        covered := state.CoveredMessageIDs()
        if len(covered) == 0 {
                // 无压缩块：仅注入 <acp> 标签
                return renderTaggedMessages(messages, state, renderTags)
        }

        // 找到第一条 user 消息索引（始终保留，即使被覆盖）
        firstUserIndex := -1
        for i, msg := range messages {
                if msg.Role == RoleUser {
                        firstUserIndex = i
                        break
                }
        }

        // 构建消息 ID → 索引映射（含已渲染 summary 的位置）
        indexByID := map[string]int{}
        summaryIndexByID := map[string]int{}
        for i, msg := range messages {
                if msg.ID == "" {
                        continue
                }
                indexByID[msg.ID] = i
                if isRenderedSummary(msg) {
                        summaryIndexByID[msg.ID] = i
                }
        }

        // 收集每个 active 块的 summary 锚点：优先用已渲染 summary 的位置（保持稳定），
        // 否则用块覆盖范围的最早消息索引
        anchors := collectSummaryAnchors(state, indexByID, summaryIndexByID)

        // 重建消息列表：在锚点位置插入 summary，跳过被覆盖的原始消息
        out := rebuildMessagesWithAnchors(messages, covered, firstUserIndex, anchors)

        // 注入 <acp> 标签（仅非 summary 消息）
        return renderTaggedMessages(out, state, renderTags)
}

// summaryIDPrefix summary 消息 ID 前缀（对齐 acp-kernel SUMMARY_ID_PREFIX）。
// 经此前缀识别"已渲染的 summary"，下次 prune 时保持其位置稳定。
const summaryIDPrefix = "acp_summary_"

// summaryMessageId 生成块的 summary 消息 ID（对齐 acp-kernel summaryMessageId）。
func summaryMessageID(blockID string) string {
        return summaryIDPrefix + blockID
}

// isRenderedSummary 报告消息是否是已渲染的 summary（对齐 acp-kernel isRenderedSummaryMessage）。
// 用于下次 prune 时保持 summary 位置稳定。
func isRenderedSummary(msg CoreMessage) bool {
        return msg.Role == RoleSystem &&
                msg.ContentType == ContentTypeText &&
                len(msg.ID) > len(summaryIDPrefix) &&
                msg.ID[:len(summaryIDPrefix)] == summaryIDPrefix
}

// summaryAnchor 一个 summary 的插入锚点。
type summaryAnchor struct {
        blockID  string
        summary  string
        topic    string
        insertAt int
}

// collectSummaryAnchors 收集每个 active 块的 summary 锚点（对齐 acp-kernel collectSummaryAnchors）。
// 优先用已渲染 summary 的位置（保持稳定）；否则用块覆盖范围的最早消息索引。
func collectSummaryAnchors(state *CompressionState, indexByID, summaryIndexByID map[string]int) []summaryAnchor {
        var anchors []summaryAnchor
        for _, b := range state.ActiveBlocks() {
                // 优先：已渲染 summary 的位置（前缀稳定的关键）
                if existingIdx, ok := summaryIndexByID[summaryMessageID(b.BlockID)]; ok {
                        anchors = append(anchors, summaryAnchor{
                                blockID:  b.BlockID,
                                summary:  b.Summary,
                                topic:    b.Topic,
                                insertAt: existingIdx,
                        })
                        continue
                }
                // 回退：块覆盖范围的最早消息索引
                earliest := -1
                for _, id := range b.EffectiveMessageIDs {
                        if idx, ok := indexByID[id]; ok && (earliest < 0 || idx < earliest) {
                                earliest = idx
                        }
                }
                if earliest < 0 {
                        earliest = 0
                }
                anchors = append(anchors, summaryAnchor{
                        blockID:  b.BlockID,
                        summary:  b.Summary,
                        topic:     b.Topic,
                        insertAt: earliest,
                })
        }
        // 按插入位置升序排序（稳定）
        for i := 1; i < len(anchors); i++ {
                for j := i; j > 0 && anchors[j-1].insertAt > anchors[j].insertAt; j-- {
                        anchors[j-1], anchors[j] = anchors[j], anchors[j-1]
                }
        }
        return anchors
}

// rebuildMessagesWithAnchors 重建消息列表：在锚点位置插入 summary，跳过被覆盖的原始消息。
// 对齐 acp-kernel rebuildMessages。
func rebuildMessagesWithAnchors(messages []CoreMessage, covered map[string]bool, firstUserIndex int, anchors []summaryAnchor) []CoreMessage {
        result := make([]CoreMessage, 0, len(messages)+len(anchors))
        anchoredSummaryIDs := map[string]bool{}
        for _, a := range anchors {
                anchoredSummaryIDs[summaryMessageID(a.blockID)] = true
        }

        pending := anchors
        for index := 0; index < len(messages); index++ {
                // 在锚点位置插入 summary（按 insertAt 顺序）
                for len(pending) > 0 && pending[0].insertAt == index {
                        result = append(result, renderSummary(pending[0]))
                        pending = pending[1:]
                }
                // 第一条 user 消息始终保留（即使被覆盖）
                if index == firstUserIndex && firstUserIndex >= 0 {
                        result = append(result, messages[index])
                        continue
                }
                // 跳过被覆盖的原始消息
                if covered[messages[index].ID] {
                        continue
                }
                // 跳过旧的 summary 副本（已被上面新渲染的替换）
                if isRenderedSummary(messages[index]) && anchoredSummaryIDs[messages[index].ID] {
                        continue
                }
                result = append(result, messages[index])
        }
        // 末尾剩余的 summary（锚点超出消息列表末尾）
        for len(pending) > 0 {
                result = append(result, renderSummary(pending[0]))
                pending = pending[1:]
        }
        return result
}

// renderSummary 渲染一个 summary 锚点为 system 消息（对齐 acp-kernel renderSummary）。
// 用 system 角色而非 assistant：system 消息稳定，provider 不会因角色变化重排缓存。
func renderSummary(a summaryAnchor) CoreMessage {
        body := a.summary
        // 去除首尾空白（对齐 acp-kernel anchor.summary.trim()）
        for len(body) > 0 && (body[0] == ' ' || body[0] == '\n' || body[0] == '\t' || body[0] == '\r') {
                body = body[1:]
        }
        for len(body) > 0 && (body[len(body)-1] == ' ' || body[len(body)-1] == '\n' || body[len(body)-1] == '\t' || body[len(body)-1] == '\r') {
                body = body[:len(body)-1]
        }
        topicLine := "[Compressed conversation section]"
        if a.topic != "" {
                topicLine = "[Compressed conversation section] — " + a.topic
        }
        text := topicLine
        if body != "" {
                text = topicLine + "\n" + body
        }
        return CoreMessage{
                ID:          summaryMessageID(a.blockID),
                Role:        RoleSystem,
                ContentType: ContentTypeText,
                Text:        text,
        }
}

// renderTaggedMessages 给消息列表注入 <acp> 标签（跳过 summary 消息，避免标签污染摘要文本）。
func renderTaggedMessages(messages []CoreMessage, state *CompressionState, renderTags bool) []CoreMessage {
        if !renderTags {
                return messages
        }
        out := make([]CoreMessage, len(messages))
        for i, msg := range messages {
                if isRenderedSummary(msg) || msg.ID == "" {
                        out[i] = msg // summary 或无 ID 消息不加标签
                        continue
                }
                out[i] = renderTaggedMessage(msg, state, renderTags)
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
