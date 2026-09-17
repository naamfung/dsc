package acp

import (
	"encoding/json"
	"strings"
)

// hide-compress-calls 节点（对齐 acp-kernel hide-consumed.ts）：
// 把「已被块消费」的 compress 工具调用对隐藏、孤儿调用只保留最新两对、
// 存活调用的 args 中重复 summary 压缩为 200 字符存根——历史 compress 调用
// 的 args 全文重复了每个范围的 summary 文本（渲染 summary 消息已携带），
// 长会话仅重复即实测 ~22K token（上游 billion-context-pi #336）。

// keepLastOrphaned 孤儿 compress 调用保留的最新对数（上游 #9：3,849 条
// 相同调用风暴 5h13m——失败必须可观察，但残留必须封顶）。
const keepLastOrphaned = 2

// summaryStubChars 存活调用 args 内 summary 的存根长度（留头便于回忆）。
const summaryStubChars = 200

// rangeKey 范围键（startRef::endRef）。
func rangeKey(startRef, endRef string) string { return startRef + "::" + endRef }

// HideConsumedResult 隐藏结果。
type HideConsumedResult struct {
	Messages []CoreMessage `json:"messages"`
	Hidden   int           `json:"hidden"`
}

// compressCallText compress 工具调用的文本载体：assistant 消息 Text 为
// args JSON（adapter 保真）；定位首个 { 解析（容忍前置渲染标签）。
type parsedCallText struct {
	prefix  string
	obj     map[string]json.RawMessage
	content []map[string]json.RawMessage
}

func parseCallText(text string) *parsedCallText {
	start := strings.Index(text, "{")
	if start < 0 {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text[start:]), &obj); err != nil {
		return nil
	}
	raw, ok := obj["content"]
	if !ok {
		return nil
	}
	var content []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &content); err != nil {
		// 非严格 provider 可能把 content 数组字符串化（上游 #230）——解一层
		var inner string
		if err := json.Unmarshal(raw, &inner); err != nil {
			return nil
		}
		if err := json.Unmarshal([]byte(inner), &content); err != nil {
			return nil
		}
	}
	if len(content) == 0 {
		return nil
	}
	return &parsedCallText{prefix: text[:start], obj: obj, content: content}
}

// entryRangeKey 取条目的范围键（startId/endId 或 messageId）。
func entryRangeKey(entry map[string]json.RawMessage) string {
	get := func(keys ...string) string {
		for _, k := range keys {
			var s string
			if raw, ok := entry[k]; ok {
				if err := json.Unmarshal(raw, &s); err == nil {
					return s
				}
			}
		}
		return ""
	}
	start := get("startId", "messageId")
	end := get("endId", "messageId")
	return rangeKey(start, end)
}

// compactEntry 把超过存根长度的 summary 压缩为存根（其余字段原样）。
func compactEntry(entry map[string]json.RawMessage) (map[string]json.RawMessage, bool) {
	raw, ok := entry["summary"]
	if !ok {
		return entry, false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil || len(s) <= summaryStubChars {
		return entry, false
	}
	stub := clampPrefix(s, summaryStubChars-1) + "…"
	out := map[string]json.RawMessage{}
	for k, v := range entry {
		out[k] = v
	}
	enc, _ := json.Marshal(stub)
	out["summary"] = enc
	return out, true
}

// rewriteCompressText 重写存活调用 args：只保留仍存活的范围 + summary 存根。
// 返回空串表示无需/无法改写（原样保留）。
func rewriteCompressText(text string, liveKeys map[string]bool) string {
	parsed := parseCallText(text)
	if parsed == nil {
		return ""
	}
	var kept []map[string]json.RawMessage
	for _, entry := range parsed.content {
		if liveKeys[entryRangeKey(entry)] {
			kept = append(kept, entry)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	return serializeCompacted(parsed, kept)
}

// compactCompressText 存活调用的 summary 存根化（范围集合不变）。
func compactCompressText(text string) string {
	parsed := parseCallText(text)
	if parsed == nil {
		return ""
	}
	changed := false
	kept := make([]map[string]json.RawMessage, 0, len(parsed.content))
	for _, entry := range parsed.content {
		e, c := compactEntry(entry)
		if c {
			changed = true
		}
		kept = append(kept, e)
	}
	if !changed {
		return ""
	}
	return serializeCompacted(parsed, kept)
}

// serializeCompacted 按原形状序列化（content 数组原为字符串则保持字符串）。
// 每个条目先过 compactEntry——重写与存根化共用同一出口（对齐上游）。
func serializeCompacted(parsed *parsedCallText, kept []map[string]json.RawMessage) string {
	compacted := make([]map[string]json.RawMessage, 0, len(kept))
	for _, entry := range kept {
		e, _ := compactEntry(entry)
		compacted = append(compacted, e)
	}
	// 检测原 content 是否为字符串化数组：obj.content 原始 JSON 以 " 开头
	rawContent, _ := parsed.obj["content"]
	stringified := len(rawContent) > 0 && rawContent[0] == '"'
	var contentOut any = compacted
	if stringified {
		enc, _ := json.Marshal(compacted)
		contentOut = string(enc)
	}
	out := map[string]json.RawMessage{}
	for k, v := range parsed.obj {
		out[k] = v
	}
	enc, err := json.Marshal(contentOut)
	if err != nil {
		return ""
	}
	out["content"] = enc
	full, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return parsed.prefix + string(full)
}

// HideConsumedCompressCalls 隐藏/压缩已消费的 compress 调用（对齐 hideConsumedCompressCalls）。
func HideConsumedCompressCalls(state *CompressionState, messages []CoreMessage) HideConsumedResult {
	allBlockCallIDs := map[string]bool{}
	activeCallIDs := map[string]bool{}
	liveKeysByCallID := map[string]map[string]bool{}
	legacyLiveByCallID := map[string]bool{}
	for i := range state.Blocks {
		b := &state.Blocks[i]
		if b.CompressCallID == "" {
			continue
		}
		allBlockCallIDs[b.CompressCallID] = true
		if !b.Active {
			continue
		}
		activeCallIDs[b.CompressCallID] = true
		if b.StartRef == "" || b.EndRef == "" {
			legacyLiveByCallID[b.CompressCallID] = true
			continue
		}
		keys, ok := liveKeysByCallID[b.CompressCallID]
		if !ok {
			keys = map[string]bool{}
			liveKeysByCallID[b.CompressCallID] = keys
		}
		keys[rangeKey(b.StartRef, b.EndRef)] = true
	}

	// 孤儿调用：无任何块引用其 callId——只保留最新 N 对（失败可观察 + 残留封顶）
	var lastOrphaned []string
	for i := len(messages) - 1; i >= 0 && len(lastOrphaned) < keepLastOrphaned; i-- {
		m := messages[i]
		if m.ToolName != "compress" || m.ContentType != ContentTypeToolCall || m.ToolCallID == "" {
			continue
		}
		if !allBlockCallIDs[m.ToolCallID] {
			lastOrphaned = append(lastOrphaned, m.ToolCallID)
		}
	}
	keepCallIDs := map[string]bool{}
	for id := range activeCallIDs {
		keepCallIDs[id] = true
	}
	for _, id := range lastOrphaned {
		keepCallIDs[id] = true
	}

	hiddenCallIDs := map[string]bool{}
	for _, m := range messages {
		if m.ToolName == "compress" && m.ContentType == ContentTypeToolCall &&
			m.ToolCallID != "" && !keepCallIDs[m.ToolCallID] {
			hiddenCallIDs[m.ToolCallID] = true
		}
	}

	hidden := 0
	result := make([]CoreMessage, 0, len(messages))
	for _, m := range messages {
		isCompressCall := m.ToolName == "compress" && m.ContentType == ContentTypeToolCall
		// 隐藏被消费调用的 call 与 result 两半
		if isCompressCall && (m.ToolCallID == "" || !keepCallIDs[m.ToolCallID]) {
			hidden++
			continue
		}
		if m.ContentType == ContentTypeToolResult && m.ToolCallID != "" && hiddenCallIDs[m.ToolCallID] {
			hidden++
			continue
		}
		// 存活调用：重写 args（剔除已消费范围 + summary 存根化）
		if isCompressCall && m.ToolCallID != "" && keepCallIDs[m.ToolCallID] {
			if keys := liveKeysByCallID[m.ToolCallID]; len(keys) > 0 && !legacyLiveByCallID[m.ToolCallID] {
				if rewritten := rewriteCompressText(m.Text, keys); rewritten != "" {
					nm := m
					nm.Text = rewritten
					result = append(result, nm)
					continue
				}
			}
			if compacted := compactCompressText(m.Text); compacted != "" {
				nm := m
				nm.Text = compacted
				result = append(result, nm)
				continue
			}
		}
		result = append(result, m)
	}
	return HideConsumedResult{Messages: result, Hidden: hidden}
}
