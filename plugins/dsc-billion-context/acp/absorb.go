package acp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// 即时工具结果吸收（对齐 acp-kernel absorb.ts）：介于「放任输出堆积到
// 50K token 才 nudge」与「10K 窗口下模型无法工作」之间的中间层。启用后，
// 每个达标的大工具结果被追加强制提示，要求模型立刻经 absorb({ref, summary})
// 蒸馏；其 tool-call + tool-result 对随后在 pre-step 被隐藏，absorb 摘要成为
// 唯一持久记录。absorb 调用本身是普通消息：常规压缩管线之后可照常折叠
// （正交设计）。

// AbsorbPromptMarker 吸收提示标记（识别已提示/已吸收）。
const AbsorbPromptMarker = "[ACP absorb]"

// ACPTeamTools ACP 管理的工具名集合（其结果不可吸收）。
var ACPManagedTools = map[string]bool{
	"compress":       true,
	"decompress":     true,
	"search_context": true,
	"acp_status":     true,
	"absorb":         true,
}

// FormatTokenCount token 数的紧凑展示（990 / 1.2K / 34K）。
func FormatTokenCount(tokens int) string {
	switch {
	case tokens < 1000:
		return itoa(tokens)
	case tokens < 10000:
		return fmt.Sprintf("%.1fK", float64(tokens)/1000)
	default:
		return itoa((tokens+500)/1000) + "K"
	}
}

// BuildAbsorbPrompt 构建追加到达标工具结果尾部的强制吸收提示。
func BuildAbsorbPrompt(ref string, tokens int, toolName string) string {
	if toolName == "" {
		toolName = "absorb"
	}
	return AbsorbPromptMarker + " This tool result (~" + FormatTokenCount(tokens) + " tokens) will be REMOVED from context. " +
		"Your IMMEDIATE next action: call " + toolName + "({ ref: \"" + ref + "\", summary: \"...\" }) — summary = distilled essentials only " +
		"(outcome, key values, exact paths:lines, error text verbatim, decisions). " +
		"Afterwards work from your summary; do NOT re-run this tool. " +
		"If the result contains nothing you need, call " + toolName + " with summary \"(nothing needed)\"."
}

// BuildAbsorbSystemPrompt 吸收启用时的 system prompt 段（对齐 buildAbsorbSystemPrompt）。
func BuildAbsorbSystemPrompt(toolName string) string {
	if toolName == "" {
		toolName = "absorb"
	}
	return "INSTANT TOOL-RESULT ABSORPTION (" + toolName + ")\n\n" +
		"Some tool results end with a " + AbsorbPromptMarker + " instruction. When you see one, your IMMEDIATE next action must be calling " +
		toolName + "({ ref, summary }) — distill that tool result's essentials into summary: outcome, key values, exact paths:lines, error text verbatim, decisions. " +
		"The original output is then removed from context; your " + toolName + " summary becomes the only durable record of it, so distill carefully. " +
		"Never call another tool or answer the user before absorbing a marked result. Do not re-run the original tool afterwards — work from your summary. " +
		toolName + " calls are ordinary context: the regular compression system may fold them later like any other message."
}

// matchToolPattern 工具名匹配：精确相等或 * 后缀通配（对齐 matchToolPattern）。
func matchToolPattern(toolName, pattern string) bool {
	if pattern == "" || toolName == "" {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(toolName, pattern[:len(pattern)-1])
	}
	return toolName == pattern
}

// isACPManagedTool 报告工具是否 ACP 管理或吸收工具自身。
func isACPManagedTool(toolName string, cfg AbsorbConfig) bool {
	if toolName == "" {
		return false
	}
	if toolName == cfg.ToolName {
		return true
	}
	return ACPManagedTools[toolName]
}

// IsAbsorbCandidate 报告工具结果消息是否在吸收提示范围内：
// 非 ACP 管理、未排除、未受保护的工具的 tool-result。
func IsAbsorbCandidate(msg CoreMessage, config Config) bool {
	if msg.ContentType != ContentTypeToolResult || msg.ToolCallID == "" {
		return false
	}
	cfg := config.Absorb
	if isACPManagedTool(msg.ToolName, cfg) {
		return false
	}
	for _, t := range config.ProtectedTools {
		if matchToolPattern(msg.ToolName, t) {
			return false
		}
	}
	for _, pattern := range cfg.ExcludeTools {
		if matchToolPattern(msg.ToolName, pattern) {
			return false
		}
	}
	return true
}

// HideAbsorbedMessages 隐藏已吸收记录覆盖的 tool-call + tool-result 对。
// 两半一起隐藏，保证 provider 可见会话结构合法；prune 的孤儿清理兜底残余。
func HideAbsorbedMessages(messages []CoreMessage, state *CompressionState) []CoreMessage {
	if len(state.Absorbed) == 0 {
		return messages
	}
	hidden := map[string]bool{}
	for _, r := range state.Absorbed {
		if r.CallMessageID != "" {
			hidden[r.CallMessageID] = true
		}
		if r.ResultMessageID != "" {
			hidden[r.ResultMessageID] = true
		}
	}
	out := messages[:0:0]
	for _, m := range messages {
		if hidden[m.ID] {
			continue
		}
		out = append(out, m)
	}
	return out
}

// AppendAbsorbPromptsResult 吸收提示追加结果。
type AppendAbsorbPromptsResult struct {
	Messages      []CoreMessage `json:"messages"`
	PromptedCount int           `json:"promptedCount"`
}

// AppendAbsorbPrompts 给可见视图中每个达标、未吸收的大工具结果追加强制提示。
// 每轮视图级文本（不持久化）：模型吸收前每轮重现，吸收后随消息对隐藏而消失。
func AppendAbsorbPrompts(messages []CoreMessage, state *CompressionState, config Config, tokenCount int) AppendAbsorbPromptsResult {
	cfg := config.Absorb
	if !cfg.Enabled {
		return AppendAbsorbPromptsResult{Messages: messages}
	}
	limit := config.ModelContextLimit
	if cfg.ContextThresholdPct > 0 && limit > 0 &&
		float64(tokenCount) < cfg.ContextThresholdPct*float64(limit) {
		return AppendAbsorbPromptsResult{Messages: messages}
	}

	absorbed := map[string]bool{}
	for _, r := range state.Absorbed {
		if r.ResultMessageID != "" {
			absorbed[r.ResultMessageID] = true
		}
	}

	prompted := 0
	out := make([]CoreMessage, len(messages))
	for i, msg := range messages {
		out[i] = msg
		if !IsAbsorbCandidate(msg, config) || absorbed[msg.ID] {
			continue
		}
		if strings.Contains(msg.Text, AbsorbPromptMarker) {
			continue
		}
		minTokens := cfg.MinToolTokens
		if minTokens <= 0 {
			minTokens = 1000
		}
		tokens := estimateTokensForText(msg.Text)
		if tokens < minTokens {
			continue
		}
		ref := RefForRaw(msg.ID, state)
		if ref == "" || ref == BLOCKED_REF {
			continue
		}
		prompted++
		out[i].Text = msg.Text + "\n\n" + BuildAbsorbPrompt(ref, tokens, cfg.ToolName)
	}
	return AppendAbsorbPromptsResult{Messages: out, PromptedCount: prompted}
}

// ParsedAbsorb 宽松解析后的 absorb 调用参数。
type ParsedAbsorb struct {
	Ref          string `json:"ref"`
	Summary      string `json:"summary"`
	AbsorbCallID string `json:"absorbCallId,omitempty"`
}

// ParseAbsorbInput 宽松解析 absorb 参数：ref 拼写 ref/messageId/of，
// summary 拼写 summary/content，兼容字符串化 JSON 载荷。
func ParseAbsorbInput(input json.RawMessage, callID string) *ParsedAbsorb {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(input, &obj); err != nil {
		return nil
	}
	pick := func(keys ...string) string {
		for _, k := range keys {
			var s string
			if raw, ok := obj[k]; ok {
				if err := json.Unmarshal(raw, &s); err == nil {
					return s
				}
			}
		}
		return ""
	}
	ref := pick("ref", "messageId", "of")
	summary := pick("summary", "content")
	if ref == "" || summary == "" {
		return nil
	}
	return &ParsedAbsorb{Ref: strings.TrimSpace(ref), Summary: summary, AbsorbCallID: callID}
}

// AbsorbOutcome absorb 调用的应用结果。
type AbsorbOutcome struct {
	State      *CompressionState `json:"-"`
	OK         bool              `json:"ok"`
	ResultText string            `json:"resultText"`
}

// ApplyAbsorb 应用模型发起的 absorb 调用：校验目标 tool-result、记录吸收。
// 消息对的隐藏发生在下一轮 pre-step（hideAbsorbedMessages），绝不在本轮中途。
func ApplyAbsorb(ref, summary, absorbCallID string, messages []CoreMessage, state *CompressionState, config Config) AbsorbOutcome {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return AbsorbOutcome{State: state, ResultText: "absorb failed: summary is empty — provide the distilled key info of the tool result."}
	}
	cfg := config.Absorb
	rawID := RawForRef(ref, state)
	if rawID == "" {
		return AbsorbOutcome{State: state, ResultText: "absorb failed: ref " + ref + " does not exist in this session (it may be hidden, already compressed, or stale)."}
	}
	for _, r := range state.Absorbed {
		if r.ResultMessageID == rawID {
			return AbsorbOutcome{State: state, OK: true, ResultText: "already absorbed (" + ref + ") — no change."}
		}
	}
	var target *CoreMessage
	for i := range messages {
		if messages[i].ID == rawID {
			target = &messages[i]
			break
		}
	}
	if target == nil {
		return AbsorbOutcome{State: state, ResultText: "absorb failed: ref " + ref + " is not visible in this session (hidden or compressed)."}
	}
	if target.ContentType != ContentTypeToolResult {
		return AbsorbOutcome{State: state, ResultText: "absorb failed: ref " + ref + " is a " + string(target.ContentType) + ", not a tool result."}
	}
	if isACPManagedTool(target.ToolName, cfg) {
		return AbsorbOutcome{State: state, ResultText: "absorb failed: " + target.ToolName + " is an ACP-managed tool result — it is not absorbable."}
	}
	for _, t := range config.ProtectedTools {
		if matchToolPattern(target.ToolName, t) {
			return AbsorbOutcome{State: state, ResultText: "absorb failed: " + target.ToolName + " is a protected tool — its results must stay visible."}
		}
	}
	if target.ToolCallID == "" {
		return AbsorbOutcome{State: state, ResultText: "absorb failed: ref " + ref + " has no tool-call id — cannot pair it for hiding."}
	}

	callID := ""
	for _, m := range messages {
		if m.ContentType == ContentTypeToolCall && m.ToolCallID == target.ToolCallID {
			callID = m.ID
			break
		}
	}
	tokens := estimateTokensForText(target.Text)
	summaryTokens := estimateTokensForText(summary)

	record := AbsorbRecord{
		ToolCallID:      target.ToolCallID,
		CallMessageID:   callID,
		ResultMessageID: target.ID,
		AbsorbCallID:    absorbCallID,
		Summary:         summary,
		TokensReclaimed: tokens,
		CreatedAt:       nowMillis(),
	}
	state.Absorbed = append(state.Absorbed, record)
	state.Stats.AbsorbedTokens += tokens

	resultText := "absorbed " + ref + " (~" + FormatTokenCount(tokens) + " tokens → summary ~" + FormatTokenCount(summaryTokens) + "). The original tool output is now hidden; your summary is the durable record."
	if tokens > 0 && summaryTokens >= tokens {
		resultText += " WARNING: your summary (~" + FormatTokenCount(summaryTokens) + " tokens) is not smaller than the original (~" + FormatTokenCount(tokens) + " tokens) — distill harder next time."
	}
	return AbsorbOutcome{State: state, OK: true, ResultText: resultText}
}

// sortAbsorbRecords 按创建时间升序（保持记录顺序稳定，供持久化）。
func sortAbsorbRecords(records []AbsorbRecord) {
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt < records[j].CreatedAt })
}
