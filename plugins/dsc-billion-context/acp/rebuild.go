package acp

import (
	"encoding/json"
	"strings"
)

// fork/恢复 state 重建（对齐 README 留待后续「fork/恢复时 state 重建（经
// session 事件日志重放）」）：消息列表本身即 append-only 事件日志——
// compress 工具调用（args 携带范围与摘要）+ 其 tool-result（结果 JSON 携带
// blocksCreated）构成可重放的压缩事件流。
//
// 重建协议：
//  1. 空白 state 上按消息顺序重新分配 ref（refs 按首次见到顺序分配，原始
//     历史完整时重放映射与原会话逐位一致）；
//  2. 依序重放每条 compress 调用：解析 args 范围 → 找到匹配 callId 的
//     tool-result → 结果 ok 且 blocksCreated>0 时按原摘要重放 ApplyCompression；
//  3. 块的 CompressCallID 随重放落位（hide-compress-calls 因此即刻可用）。
//
// 触发时机（main.go handlePreStep）：state 为空白（无块、零压缩计数）而
// 历史中已存在 ACP 压缩痕迹（渲染 summary 消息或 compress 调用）——典型场景：
// 状态文件缺失/损坏、会话 fork 到新 sessionID、跨机恢复。尽力而为：紧急压缩
//（request-error 路径）不在历史中，重放后其覆盖关系丢失属可接受降级。

// HasACPStateTrace 报告历史中是否存在 ACP 压缩痕迹（summary 消息或 compress 调用）。
func HasACPStateTrace(messages []CoreMessage) bool {
	for i := range messages {
		m := &messages[i]
		if m.ContentType == ContentTypeToolCall && m.ToolName == "compress" {
			return true
		}
		if isRenderedSummary(*m) {
			return true
		}
	}
	return false
}

// compressCallRecord 历史中的一条 compress 调用与其结果。
type compressCallRecord struct {
	callID  string
	ranges  []PruneRange
	succeed bool
}

// ParseCompressArgs 解析 compress 调後 args 的 content 范围集（重建与
// callId 回塔共用）。
func ParseCompressArgs(text string) []PruneRange {
	start := strings.Index(text, "{")
	if start < 0 {
		return nil
	}
	var req struct {
		Content []struct {
			StartRef string `json:"startId"`
			EndRef   string `json:"endId"`
			Summary  string `json:"summary"`
			Topic    string `json:"topic"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(text[start:]), &req); err != nil {
		return nil
	}
	var ranges []PruneRange
	for _, r := range req.Content {
		if r.StartRef == "" || r.EndRef == "" || r.Summary == "" {
			continue
		}
		ranges = append(ranges, PruneRange{StartRef: r.StartRef, EndRef: r.EndRef, Summary: r.Summary, Topic: r.Topic})
	}
	return ranges
}

// compressResultOK 解析 tool-result JSON，报告 blocksCreated>0。
func compressResultOK(text string) bool {
	start := strings.Index(text, "{")
	if start < 0 {
		return false
	}
	var res struct {
		OK            bool `json:"ok"`
		BlocksCreated int  `json:"blocksCreated"`
	}
	if err := json.Unmarshal([]byte(text[start:]), &res); err != nil {
		return false
	}
	return res.OK && res.BlocksCreated > 0
}

// RebuildStateFromMessages 从消息列表重放压缩事件流重建 state。
// 返回重建后的 state 与重放成功的压缩次数。
func RebuildStateFromMessages(messages []CoreMessage, config Config) (*CompressionState, int) {
	state := CreateInitialState()
	AssignRefs(messages, state)

	// 收集 compress 调用与其 tool-result（callID → 结果文本）
	results := map[string]string{}
	for i := range messages {
		m := &messages[i]
		if m.ContentType == ContentTypeToolResult && m.ToolCallID != "" && m.Text != "" {
			results[m.ToolCallID] = m.Text
		}
	}

	replayed := 0
	for i := range messages {
		m := &messages[i]
		if m.ContentType != ContentTypeToolCall || m.ToolName != "compress" || m.ToolCallID == "" {
			continue
		}
		ranges := ParseCompressArgs(m.Text)
		if len(ranges) == 0 {
			continue
		}
		if !compressResultOK(results[m.ToolCallID]) {
			continue
		}
		_, result := ApplyCompression(ranges, messages, state, config)
		if result.BlocksCreated > 0 {
			replayed += result.BlocksCreated
			// 回填本次调用的 callId 到新产出的块（同 runID 的最新块）
			for j := range state.Blocks {
				b := &state.Blocks[j]
				if b.CompressCallID == "" && b.RunID == state.Blocks[len(state.Blocks)-1].RunID {
					b.CompressCallID = m.ToolCallID
				}
			}
		}
	}
	return state, replayed
}
