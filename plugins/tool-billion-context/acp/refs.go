package acp

import "sort"

// AssignRefs 为消息列表分配 mNNNNN ref（对齐 acp-kernel assign-refs）。
// 已有 ref 的消息保留；新消息按顺序分配下一个 ref。
// 返回更新后的 state（state.MessageRefs 被修改）。
//
// ref 是模型在 compress 调用中引用消息的稳定标识（如 m00005）。
// 一旦分配，ref 永不改变（即使消息被压缩/恢复）。
func AssignRefs(messages []CoreMessage, state *CompressionState) {
	for _, msg := range messages {
		if msg.ID == "" {
			continue
		}
		if _, has := state.MessageRefs.ByRaw[msg.ID]; has {
			continue
		}
		ref := fmtRef(len(state.MessageRefs.ByRaw))
		state.MessageRefs.ByRaw[msg.ID] = ref
		state.MessageRefs.ByRef[ref] = msg.ID
	}
}

// HighestUsedIndex 返回当前已分配的最大 ref 索引（用于续编）。
func HighestUsedIndex(state *CompressionState) int {
	maxIdx := -1
	for _, ref := range state.MessageRefs.ByRaw {
		if idx, ok := parseRefIdx(ref); ok && idx > maxIdx {
			maxIdx = idx
		}
	}
	return maxIdx
}

// RefForRaw 返回 raw id 对应的 ref（不存在返回空串）。
func RefForRaw(rawID string, state *CompressionState) string {
	return state.MessageRefs.ByRaw[rawID]
}

// RawForRef 返回 ref 对应的 raw id（不存在返回空串）。
func RawForRef(ref string, state *CompressionState) string {
	return state.MessageRefs.ByRef[ref]
}

// parseRefIdx 解析 m00005 形式的 ref，返回数字部分（5）。
func parseRefIdx(ref string) (int, bool) {
	if len(ref) < 2 || ref[0] != 'm' {
		return 0, false
	}
	n := 0
	for i := 1; i < len(ref); i++ {
		c := ref[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// SortedRefs 返回按 ref 数字升序排序的 ref 列表（稳定迭代，避免 map 随机序）。
func SortedRefs(state *CompressionState) []string {
	out := make([]string, 0, len(state.MessageRefs.ByRef))
	for ref := range state.MessageRefs.ByRef {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool {
		ai, _ := parseRefIdx(out[i])
		aj, _ := parseRefIdx(out[j])
		return ai < aj
	})
	return out
}

// BLOCKED_REF 表示该 ref 已被压缩块遮蔽（不再可见于消息列表）。
const BLOCKED_REF = "m-blocked"
