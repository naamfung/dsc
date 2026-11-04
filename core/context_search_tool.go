package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dsc/proto"
	"dsc/session"
)

// 会话历史检索工具（宿主内置，只读）。
//
// 存在的理由：小窗口读超长会话时，滑窗会把早期记录从**本次请求**里裁掉（裁剪只
// 影响这一发，会话日志一字不改）。模型从此看不到那部分内容——它要么凭印象猜，
// 要么重新去读文件/跑命令。这个工具把「整份会话日志」变成可检索的：按关键词找回
// 被滑窗裁掉的原话、命令与输出。
//
// 与记忆插件的分工：记忆是跨会话、面向「值得长期记住的事实」，经过模型摘要；
// 这里检索的是**当前会话的完整原始记录**，不依赖任何摘要，是一次精确回溯。
//
// 为什么由宿主提供、而不是某个插件：数据在宿主（会话日志归宿主所有），且它对
// 「窗口是否装得下历史」这件事有唯一的判断权。
const contextSearchToolName = "context_search"

// contextSearchDefaultLimit / contextSearchMaxLimit 单次返回的命中条数。
// 上限存在是为了别让一次检索把整段历史重新灌回窗口——那就等于没裁剪。
const (
	contextSearchDefaultLimit = 8
	contextSearchMaxLimit     = 20
	contextSearchSnippetRunes = 240
	// 按 id 取回完整消息时的余量控制。
	//
	// 不设死字节上限：能读多少由**当前上下文还剩多少**决定——窗口 200K、已用 20K 时
	// 一条 3 万字符的工具输出本该整段读到，写死 8000 会逼模型反复分段甚至换别的工具
	// 绕路，那才是真浪费。反过来窗口 8K 时也必须有下限，否则「取回」只会回一句空话：
	// 这时给一个最小配额，让宿主滑窗去腾地方。
	contextSearchMinFetchTokens = 1000
	// 预留比例与下限：取回的内容马上会成为下一步请求的一部分，得给模型的下一步
	// 输出和后续工具结果留位置，不能贴着窗口顶棚把余量一次性用光。
	contextSearchReserveRatio       = 8    // 1/8 窗口
	contextSearchReserveMin         = 2000 // 但至少留这么多 token
	contextSearchUnknownWindowChars = 8000 // 窗口未知时的兜底字符数
)

type contextSearchTool struct{ m *Manager }

func (t *contextSearchTool) Name() string { return contextSearchToolName }

func (t *contextSearchTool) Description() string {
	return "Search this session's FULL recorded history (every user/assistant message and tool output, including the parts the sliding window has dropped from your context) for a keyword or phrase. " +
		"Use it when you need something that happened earlier but is no longer visible: an earlier decision, a file path, a command you ran, an error message, a value someone gave you. " +
		"Multiple words are ANDed (all must appear in the same message). Each hit is one message — not a whole turn and not a whole step — and comes back as a short snippet plus its id and position in the log. " +
		"To read one of them in full, call this tool again with message_id=<that id>: it returns that single message verbatim, as much as the remaining context allows — it is only split when the message does not fit, and then the reply gives the offset to continue. " +
		"This scans the whole session log, so keep queries specific."
}

func (t *contextSearchTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "Keyword or phrase to look for. Multiple space-separated words must all appear in the same message. Required unless message_id is given."},
    "limit": {"type": "integer", "description": "Maximum number of matches to return (default 8, max 20)."},
    "message_id": {"type": "string", "description": "Return this one message in full instead of searching. Use the id printed with a hit (e.g. event-3921). message_id and query are alternatives; when both are given, message_id wins."},
    "max_chars": {"type": "integer", "description": "When fetching one message: optional cap on characters returned, for when you only want the beginning. Omit it to get the whole message whenever it fits the remaining context — do not use it to keep fetches small, a needlessly small fetch wastes another call later."},
    "offset": {"type": "integer", "description": "When fetching one message: character offset to continue from a fetch that was truncated (the reply names the exact value)."}
  },
  "additionalProperties": false
}`)
}

func (t *contextSearchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Query     string `json:"query"`
		Limit     int    `json:"limit"`
		MessageID string `json:"message_id"`
		MaxChars  int    `json:"max_chars"`
		Offset    int    `json:"offset"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	msgID := strings.TrimSpace(in.MessageID)
	terms := strings.Fields(strings.ToLower(in.Query))
	if len(terms) == 0 && msgID == "" {
		return "", fmt.Errorf("query is empty: provide query to search, or message_id to read one message in full")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = contextSearchDefaultLimit
	}
	if limit > contextSearchMaxLimit {
		limit = contextSearchMaxLimit
	}

	sess, err := t.m.loadSessionForSearch(CallerFromContext(ctx))
	if err != nil {
		return "", err
	}
	if sess == nil {
		return "(no session log found to search)", nil
	}
	msgs := sess.DeriveMessages()
	if len(msgs) == 0 {
		return "(this session has no recorded messages yet)", nil
	}
	// 取回模式：只认 id，不做检索——两者是不同意图，同时给时以取回为准。
	if msgID != "" {
		return t.fetchMessage(ctx, msgs, msgID, in.MaxChars, in.Offset)
	}

	type hit struct {
		idx     int
		id      string
		role    string
		snippet string
	}
	var hits []hit
	total := 0
	for i, m := range msgs {
		if !matchesAllTerms(searchHaystack(m), terms) {
			continue
		}
		total++
		if len(hits) < limit {
			hits = append(hits, hit{i, m.GetId(), m.GetRole(), snippetAround(m.GetContent(), terms, contextSearchSnippetRunes)})
		}
	}
	if total == 0 {
		return fmt.Sprintf("No match for %q in %d recorded messages (searched the whole session log).", in.Query, len(msgs)), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d match(es) for %q in %d recorded messages; showing %d. "+
		"These come from the full session log, not from the (possibly trimmed) window:\n",
		total, in.Query, len(msgs), len(hits))
	for _, h := range hits {
		fmt.Fprintf(&b, "\n[#%d %s %s] %s\n", h.idx, h.id, h.role, h.snippet)
	}
	if total > len(hits) {
		fmt.Fprintf(&b, "\n(%d more match(es) not shown — narrow the query or raise limit, max %d.)\n",
			total-len(hits), contextSearchMaxLimit)
	}
	// 片段只是指路：要看原文就按 id 取回那一条（不是整步、也不是整轮）。
	fmt.Fprintf(&b, "\n(For the full text of any hit: call again with message_id=<id>, e.g. message_id=%q.)", hits[0].id)
	return b.String(), nil
}

// fetchMessage 按 id 返回单条消息的正文，能读多少取决于**当前上下文余量**。
//
// 单位明确是「一条派生消息」：取一条 tool 结果不会带出同一步的其它工具输出，取一条
// assistant 也不会带出下一步。返回的正文是**派生视图**里那一份（工具结果已做 TOON
// 规范化）——与检索匹配的文本同一份，所以片段和全文不会互相矛盾。
//
// 为什么上限来自余量而不是常量：写死的字节数与小窗口/大窗口都对不上——小窗口该缩，
// 大窗口却白白截断。模型要的是一条完整消息，条件允许就应整段给；条件不允许才分段，
// 并把「还剩多少」说清楚，让它自己决定是续读还是换个办法。
func (t *contextSearchTool) fetchMessage(ctx context.Context, msgs []*proto.Message, id string, maxChars, offset int) (string, error) {
	var msg *proto.Message
	for _, m := range msgs {
		if m.GetId() == id {
			msg = m
			break
		}
	}
	if msg == nil {
		return "", fmt.Errorf("message %q not found in this session's history (ids look like event-3921; take one from a search result)", id)
	}
	full := []rune(msg.GetContent())
	total := len(full)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}

	budget := t.fetchBudgetTokens(CallerFromContext(ctx), msgs)
	available := total - offset
	count := fitRunes(full[offset:], budget)
	budgetLimited := count < available // 本次截断是上下文余量造成的
	if maxChars > 0 && maxChars < count {
		count = maxChars // 模型自己要求更少：尊重，且不是「放不下」
		budgetLimited = false
	}
	end := offset + count

	var b strings.Builder
	fmt.Fprintf(&b, "[%s %s · %d chars total · showing %d-%d]", msg.GetId(), msg.GetRole(), total, offset, end)
	if budget > 0 {
		fmt.Fprintf(&b, " · ≈%d tokens of request headroom left", budget)
	}
	b.WriteString("\n")
	b.WriteString(string(full[offset:end]))
	if offset == 0 {
		// 工具调用只能在同一份消息对象里，正文之外单独列出；只在第一段给，续读时不重复。
		if calls := msg.GetToolCalls(); len(calls) > 0 {
			b.WriteString("\n\n[tool calls in this message]")
			for _, tc := range calls {
				fmt.Fprintf(&b, "\n- %s %s", tc.GetName(), tc.GetArgumentsJson())
			}
		}
	}
	if end < total {
		reason := "requested max_chars"
		if budgetLimited {
			reason = "the rest does not fit the remaining context headroom"
		}
		fmt.Fprintf(&b, "\n\n[truncated at %d of %d chars — %s; call again with message_id=%q offset=%d to read on]",
			end, total, reason, id, end)
	}
	return b.String(), nil
}

// fetchBudgetTokens 计算这次取回还能占用多少 token。
//
// **口径只有一个**：与宿主裁窗（enforceRequestCeiling）完全同一套——同样的
// EstimateProtoMessagesTokens/EstimateProtoToolTokens 估算、同样的 contextCeilingRatio
// 上限、同样的 ceilingInflation 偏差系数。不新造第三个数字（那正是「标题栏、面板、
// provider 三处对不上」的老问题）：这里算的「余量」就是裁窗开始丢历史之前还剩多少，
// 返回的正文按同一个估算器折算成字符数，所以「装得下」与「不会被裁」是同一句话。
//
// 返回 0 表示窗口未知（调用方退回保守字符数）。
func (t *contextSearchTool) fetchBudgetTokens(sessionID string, msgs []*proto.Message) int {
	if t.m == nil {
		return 0
	}
	window := t.m.ContextWindow()
	if window <= 0 {
		return 0
	}
	return t.m.requestHeadroomTokens(sessionID, msgs)
}

// requestHeadroomTokens 返回「下一次请求在下限之上还能再放多少 token」。
//
// 推导与 enforceRequestCeiling 一一对应：
//
//	允许的估算上限 limit = window × contextCeilingRatio ÷ inflation
//	当前表面 surface = 估算(消息) + 估算(工具目录)
//	余量 = limit − surface
//
// 它描述的是「再往里加这么多，宿主仍然不需要滑窗」。
func (m *Manager) requestHeadroomTokens(sessionID string, msgs []*proto.Message) int {
	if m == nil {
		return 0
	}
	window := m.ContextWindow()
	if window <= 0 {
		return 0
	}
	tools := m.AllToolsProto() // 工具目录每次请求都要发，漏算它会高估余量
	m.mu.Lock()
	model := m.modelName
	m.mu.Unlock()

	surface := EstimateProtoMessagesTokens(msgs) + EstimateProtoToolTokens(tools)
	inflation := m.ceilingInflation(sessionID, model)
	// 预算直接取收口函数的同一个数（ceilingLimit），不在别处再写一遍乘除：
	// 两处各算一次就必然在改比例/折算时漂开。
	limit := ceilingLimit(window, inflation, contextCeilingRatio)
	headroom := limit - surface
	if headroom < contextSearchMinFetchTokens {
		// 已经贴着上限（或超出）：仍给一个最小配额。宿主会滑窗腾地方，
		// 回一句「没地方了」对模型毫无用处。
		headroom = contextSearchMinFetchTokens
	}
	return headroom
}

// fitRunes 返回 text 开头能放进 budgetTokens 的最长前缀（按 rune 计）。
//
// 二分而不是按平均值折算：同一段文本里 CJK（1 token/字）与 ASCII（4 字符/token）
// 混排，平均值会让中文日志被砍掉一半、纯英文日志又留下超出预算的内容。
func fitRunes(text []rune, budgetTokens int) int {
	if len(text) == 0 {
		return 0
	}
	if budgetTokens <= 0 {
		// 窗口未知：退回一个保守的字符数，宁可少给也不要凭空塞爆窗口。
		return min(len(text), contextSearchUnknownWindowChars)
	}
	lo, hi := 0, len(text)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if EstimateTextTokens(string(text[:mid])) <= budgetTokens {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// loadSessionForSearch 按调用方会话 id 载入会话日志；不可用时退回默认（项目）会话。
// 为 nil 时返回 (nil, nil)：调用方据此给出「没有可检索的历史」而不是报错。
func (m *Manager) loadSessionForSearch(sessionID string) (*session.Session, error) {
	if m == nil {
		return nil, nil
	}
	st, err := m.sessionStore()
	if err != nil {
		return nil, err
	}
	if sessionID == "" {
		sessionID = m.DefaultSessionID()
	}
	if sessionID == "" {
		return nil, nil
	}
	return st.Load(sessionID)
}

// searchHaystack 收集一条消息里可被检索的文本：正文 + 工具调用的名称与参数。
//
// 参数也要进去：模型常常只记得「我跑过某条命令 / 改过某个文件」，那部分信息在
// tool_calls 的 arguments 里，而不在任何正文里。
func searchHaystack(m *proto.Message) string {
	if m == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.ToLower(m.GetContent()))
	for _, tc := range m.GetToolCalls() {
		b.WriteString("\n")
		b.WriteString(strings.ToLower(tc.GetName()))
		b.WriteString(" ")
		b.WriteString(strings.ToLower(tc.GetArgumentsJson()))
	}
	return b.String()
}

// matchesAllTerms 要求每个词都出现（AND）。多词时用 AND 而不是 OR：
// 检索的目的是「精确定位那一段」，OR 会让常见词把无关记录全捞回来。
func matchesAllTerms(hay string, terms []string) bool {
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

// snippetAround 取命中位置附近的一段作为摘要：折叠换行与连续空白成单空格
// （模型与 TUI 都按一行读更省事），两端加省略号表明被截断。
func snippetAround(text string, terms []string, maxRunes int) string {
	if text == "" {
		return "(no text; matched on tool arguments)"
	}
	flat := collapseWhitespace(text)
	lower := strings.ToLower(flat)
	pos := -1
	for _, t := range terms {
		if i := strings.Index(lower, t); i >= 0 && (pos < 0 || i < pos) {
			pos = i
		}
	}
	runes := []rune(flat)
	if pos < 0 || len(runes) <= maxRunes {
		return truncateRunes(flat, maxRunes)
	}
	// pos 是字节下标，换算成 rune 下标再取窗口
	runePos := len([]rune(flat[:pos]))
	start := runePos - maxRunes/3
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(runes) {
		end = len(runes)
		start = end - maxRunes
	}
	out := string(runes[start:end])
	if start > 0 {
		out = "..." + out
	}
	if end < len(runes) {
		out += "..."
	}
	return out
}

// collapseWhitespace 把换行/Tab/连续空白压成单个空格（保留词语边界）。
func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}
