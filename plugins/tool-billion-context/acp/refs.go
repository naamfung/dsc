package acp

import "sort"

// BoundaryKind 边界类型（对齐 acp-kernel BoundaryKind）。
type BoundaryKind string

const (
        BoundaryMessage BoundaryKind = "message" // mNNNNN 形式
        BoundaryBlock   BoundaryKind = "block"   // bN 形式（块引用，触发蒸馏）
)

// ParsedBoundary 解析后的边界引用（对齐 acp-kernel ParsedBoundary）。
type ParsedBoundary struct {
        Kind      BoundaryKind
        NumericID int    // message ref 的数字部分（m00005 → 5）或 block ID 数字部分（b3 → 3）
        Raw       string // 原始输入
}

// ParseBoundary 解析边界引用：mNNNNN（消息）或 bN（块）。
// 返回 nil 表示无法解析（既不是 m 也不是 b 格式）。
// 对齐 acp-kernel parseBoundary。
//
// 块引用 bN 用于触发多层蒸馏：模型在 compress 调用中用 bN 作为 startId/endId，
// 表示要把这些块蒸馏成更高层级的摘要（T1→T2 或 T2→T3）。
func ParseBoundary(ref string) *ParsedBoundary {
        if len(ref) == 0 {
                return nil
        }
        // 跳过前导空格、转小写
        normalized := toLowerASCII(trimSpaces(ref))
        if len(normalized) == 0 {
                return nil
        }
        // mNNNNN 形式（允许前导零，0 也合法——第一条消息）
        if normalized[0] == 'm' {
                n, ok := parseDigits(normalized[1:])
                if ok && n >= 0 && n <= 99999 {
                        return &ParsedBoundary{Kind: BoundaryMessage, NumericID: n, Raw: normalized}
                }
        }
        // bN 形式（块引用，0 也合法——第一个块是 b0）
        if normalized[0] == 'b' {
                n, ok := parseDigits(normalized[1:])
                if ok && n >= 0 {
                        return &ParsedBoundary{Kind: BoundaryBlock, NumericID: n, Raw: normalized}
                }
        }
        return nil
}

// parseDigits 解析纯数字字符串，返回数字与是否成功。
func parseDigits(s string) (int, bool) {
        if len(s) == 0 {
                return 0, false
        }
        n := 0
        for i := 0; i < len(s); i++ {
                c := s[i]
                if c < '0' || c > '9' {
                        return 0, false
                }
                n = n*10 + int(c-'0')
        }
        return n, true
}

// toLowerASCII 把 ASCII 大写字母转小写（避免引入 strings 包）。
func toLowerASCII(s string) string {
        out := make([]byte, len(s))
        for i := 0; i < len(s); i++ {
                c := s[i]
                if c >= 'A' && c <= 'Z' {
                        out[i] = c + 32
                } else {
                        out[i] = c
                }
        }
        return string(out)
}

// trimSpaces 去除首尾空格与制表符（避免引入 strings 包）。
func trimSpaces(s string) string {
        start, end := 0, len(s)
        for start < end && (s[start] == ' ' || s[start] == '\t') {
                start++
        }
        for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
                end--
        }
        return s[start:end]
}

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
