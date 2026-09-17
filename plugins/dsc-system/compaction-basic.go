// dsc-system 驻留：基础上下文压缩（对齐 DSH compaction-basic）。
// 自宿主 core/compaction.go 迁入——压缩策略与状态归还插件，宿主零压缩代码；
// 经通用（dsc）类型的 agent/pre-step 事件接管机制生效（与 dsc-billion-context
// 同一协议，agent 与协议零改动）：
//
//   - agent/pre-step（waterfall 拦截）：收到本步消息列表（MessagesJSON）；
//     估算用量超过阈值（默认窗口 80%，对齐 compaction-basic 压力语义；agent 经
//     [DSC_COMPACTION_BACKEND_ACTIVE] 标记检测到后端即跳过内联压缩——接管靠
//     能力声明驱动，与阈值无关）时保留尾部
//     （RetainRatio / RetainTokensMin 取大），把未压缩前段经 LLM 生成摘要
//     （interconnect 聚合 LLM；未互联或调用失败退化为截断式摘要），以
//     {"messages": [...]} 返回改写后的消息列表。
//   - agent/request-error（code=context_window_exceeded）：溢出紧急压缩——
//     以截断式摘要压缩未压缩前段（不调 LLM：溢出时再调大概率同样溢出），
//     返回 {"retry": true} 让宿主重走 pre-step（应用改写）+ 重发请求。
//
// per-session 状态：pre-step 改写只作用于本次 LLM 请求（agent 内部历史不改写），
// 逐 step 复用靠进程内 per-session 记录（内存 map）：记录已压缩前缀的指纹
// （sha256），指纹吻合直接复用既有摘要、仅对新溢出段追加压缩；历史被改写
// （指纹失配）则丢弃记录从头评估。对齐 DSH compaction-basic 的零文件 IO：
// 压缩状态是运行时兜底的内部记账，持久化由宿主会话存储承担（DSH 侧为
// session 日志事件）；插件重启即重新评估（指纹失配语义的自然延伸），
// 文件落盘是加强版（dsc-billion-context，记忆形态）的领域。
//
// 配置（DSC_ 前缀 env 白名单天然可见）：
//   - DSC_COMPACTION_BASIC_CONTEXT_WINDOW  上下文窗口 token 数（默认 131072；0 = 驻留禁用）
//   - DSC_COMPACTION_BASIC_THRESHOLD       触发阈值比例（默认 0.80，对齐 compaction-basic；
//     可调，与接管机制无关）
//   - DSC_COMPACTION_BASIC_RETAIN_RATIO    保留尾部比例（默认 0.16）
//   - DSC_COMPACTION_BASIC_RETAIN_MIN      保留尾部最少 token 数（默认 1024）
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	dsc "dsc-sdk"

	"dsc/core"
	"dsc/core/llmclient"
	"dsc/proto"
	"dsc/session"
)

// llmChat 聚合 LLM 的最小接口（*llmclient.Client 满足；单测以 fake 注入）。
type llmChat interface {
	Chat(ctx context.Context, messages []*proto.Message, maxTokens int32) (*proto.ChatResponse, error)
}

// compactionRecord 一段已压缩前缀：[0, UpTo) 被替换为 Summary。
// HeadHash 是前缀指纹——历史 append-only 增长时指纹稳定，复用摘要免重复压缩；
// 历史被改写（指纹失配）则全部作废。记录连续覆盖 [0, last.UpTo)。
type compactionRecord struct {
	UpTo      int    `json:"up_to"`               // 已压缩前缀长度（消息条数）
	HeadHash  string `json:"head_hash"`           // 前缀指纹（sha256 hex）
	Summary   string `json:"summary"`             // 摘要内容
	Shadowed  int    `json:"shadowed_tokens"`     // 被遮蔽内容的估算 token 数
	ID        string `json:"compaction_id"`       // 稳定标识（时间戳，进程内唯一）
	Emergency bool   `json:"emergency,omitempty"` // 溢出紧急压缩（截断式摘要）
}

// compactionState per-session 压缩状态（进程内内存 map 项）。
type compactionState struct {
	Records []compactionRecord `json:"records"`
}

// compactionBasicServer 基础压缩驻留（对齐宿主原 BasicCompactionEngine 语义：
// 压力驱动 + 溢出紧急，保留尾部，LLM 摘要，截断式退化，防重入）。
type compactionBasicServer struct {
	mu          sync.Mutex
	llm         llmChat // interconnect 聚合 LLM（未互联为 nil → 截断式退化）
	compacting  bool    // 防重入：同一时间只允许一次压缩
	lastMsgs    []*proto.Message
	lastSession string

	window      int     // 上下文窗口 token 数（0 = 驻留禁用）
	threshold   float64 // 压力触发阈值比例
	retainRatio float64 // 保留尾部比例
	retainMin   int     // 保留尾部最少 token 数

	states map[string]*compactionState // per-session 压缩记录（进程内，须持 mu）
}

// newCompactionBasicServer 读 env 构建驻留（fail-soft：非法配置逐项回退默认；
// window<=0 即禁用——与 timeout/spill 的 0 禁用同款语义）。
func newCompactionBasicServer() *compactionBasicServer {
	s := &compactionBasicServer{
		window:      131072,
		threshold:   0.80,
		retainRatio: 0.16,
		retainMin:   1024,
	}
	if v := os.Getenv("DSC_COMPACTION_BASIC_CONTEXT_WINDOW"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			s.window = n
		}
	}
	if v := os.Getenv("DSC_COMPACTION_BASIC_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 && f < 1 {
			s.threshold = f
		}
	}
	if v := os.Getenv("DSC_COMPACTION_BASIC_RETAIN_RATIO"); v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 && f < 1 {
			s.retainRatio = f
		}
	}
	if v := os.Getenv("DSC_COMPACTION_BASIC_RETAIN_MIN"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			s.retainMin = n
		}
	}
	s.states = map[string]*compactionState{}
	return s
}

// attachLLM 缓存 interconnect 聚合 LLM 客户端（宿主挂载后回调注入）。
// nil 不覆盖：未互联时保持 nil，摘要走截断式退化（对齐宿主引擎 nil-LLM 路径）。
func (s *compactionBasicServer) attachLLM(c *llmclient.Client) {
	if c == nil {
		return
	}
	s.mu.Lock()
	s.llm = c
	s.mu.Unlock()
}

// handleHostEvent 事件入口（main.go 的 hook 多路复用之一）。
func (s *compactionBasicServer) handleHostEvent(ctx context.Context, eventType, dataJSON string) (string, error) {
	switch eventType {
	case string(core.EventAgentPreStep):
		return s.handlePreStep(ctx, dataJSON)
	case string(core.EventAgentRequestError):
		return s.handleRequestError(ctx, dataJSON)
	default:
		return "", nil
	}
}

// handlePreStep 压力驱动压缩（agent/pre-step 拦截）。
// 返回 {"messages": [...]}（改写后列表）或 ""（本步不改写，零开销）。
func (s *compactionBasicServer) handlePreStep(ctx context.Context, dataJSON string) (string, error) {
	if s.window <= 0 {
		return "", nil // 驻留禁用
	}
	var ev core.AgentPreStepEvent
	if err := json.Unmarshal([]byte(dataJSON), &ev); err != nil {
		return "", fmt.Errorf("compaction-basic: parse pre-step event: %w", err)
	}
	var msgs []*proto.Message
	if ev.MessagesJSON != "" {
		if err := json.Unmarshal([]byte(ev.MessagesJSON), &msgs); err != nil {
			return "", fmt.Errorf("compaction-basic: parse messages: %w", err)
		}
	}
	sess := s.sessionKey(ev.Session)
	s.mu.Lock()
	s.lastMsgs = msgs // 缓存供溢出紧急压缩使用（request-error 事件不带消息列表）
	s.lastSession = sess
	st := s.stateFor(sess)
	// 指纹校验：历史被改写/回滚时丢弃记录从头评估
	records := validRecords(msgs, st.Records)
	s.mu.Unlock()
	if len(msgs) == 0 {
		return "", nil
	}

	// 复用既有压缩后的用量评估：低于阈值则直接返回改写（或无记录时不改写）
	rewritten := applyRecords(msgs, records)
	if s.estimateMsgs(rewritten) < s.pressureThreshold() {
		if len(records) > 0 {
			return rewriteJSON(rewritten)
		}
		return "", nil
	}

	// 保留尾部预算：从末尾向前累积（对齐宿主引擎语义：Ratio 与 Min 取大）
	retainIdx := s.retainBoundary(msgs)
	base := 0
	if len(records) > 0 {
		base = records[len(records)-1].UpTo
	}
	if retainIdx >= len(msgs) || retainIdx <= base {
		// 可压缩段已被既有记录覆盖（或全部落在保留区）：尽力返回当前改写；
		// 用量仍超阈值时溢出紧急压缩（agent/request-error）仍是安全阀
		if len(records) > 0 {
			return rewriteJSON(rewritten)
		}
		return "", nil
	}

	summary, id := s.summarize(ctx, msgs[base:retainIdx], false)
	rec := compactionRecord{
		UpTo:     retainIdx,
		HeadHash: fingerprint(msgs[:retainIdx]),
		Summary:  summary,
		Shadowed: s.estimateMsgs(msgs[base:retainIdx]),
		ID:       id,
	}
	s.mu.Lock()
	st.Records = append(records, rec)
	s.mu.Unlock()
	return rewriteJSON(applyRecords(msgs, st.Records))
}

// handleRequestError 溢出紧急压缩（agent/request-error 拦截）：
// code=context_window_exceeded 时以截断式摘要压缩未压缩前段（保留最后 1 条，
// 当前用户意图/在途上下文不动），返回 {"retry": true} 让宿主重走 pre-step + 重发。
// 不调 LLM：溢出时再调大概率同样溢出（对齐 dsc-billion-context 紧急路径的取舍）。
func (s *compactionBasicServer) handleRequestError(ctx context.Context, dataJSON string) (string, error) {
	if s.window <= 0 {
		return "", nil
	}
	var ev core.AgentRequestErrorEvent
	if err := json.Unmarshal([]byte(dataJSON), &ev); err != nil {
		return "", fmt.Errorf("compaction-basic: parse request-error event: %w", err)
	}
	if ev.Code != "context_window_exceeded" {
		return "", nil // 仅对上下文溢出触发；rate_limited 等不归压缩管
	}
	s.mu.Lock()
	msgs := s.lastMsgs
	sess := s.lastSession
	s.mu.Unlock()
	if len(msgs) < 2 {
		return "", nil // 无缓存消息（未走过 pre-step）无法压缩
	}
	s.mu.Lock()
	st := s.stateFor(sess)
	records := validRecords(msgs, st.Records)
	s.mu.Unlock()
	base := 0
	if len(records) > 0 {
		base = records[len(records)-1].UpTo
	}
	end := len(msgs) - 1 // 保留最后 1 条
	if end <= base {
		return "", nil // 无新可压缩段，重试同样的改写无意义
	}
	summary, id := s.summarize(ctx, msgs[base:end], true)
	rec := compactionRecord{
		UpTo:      end,
		HeadHash:  fingerprint(msgs[:end]),
		Summary:   summary,
		Shadowed:  s.estimateMsgs(msgs[base:end]),
		ID:        id,
		Emergency: true,
	}
	s.mu.Lock()
	st.Records = append(records, rec)
	s.mu.Unlock()
	return `{"retry": true}`, nil
}

// summarize 生成 [region] 的摘要。LLM 路径：聚合 LLM 生成（对齐宿主引擎提示词与
// maxTokens=窗口/8、下限 512）；LLM 未互联或失败：退化为截断式摘要（对齐宿主引擎
// nil-LLM 路径）。emergency=true 强制截断式（溢出时不赌 LLM 可用）。
func (s *compactionBasicServer) summarize(ctx context.Context, region []*proto.Message, emergency bool) (string, string) {
	id := fmt.Sprintf("compact-%d", time.Now().UnixMilli())
	if !emergency {
		s.mu.Lock()
		llm := s.llm
		busy := s.compacting
		if llm != nil && !busy {
			s.compacting = true
		}
		s.mu.Unlock()
		if llm != nil && !busy {
			summary, err := s.llmSummary(ctx, llm, region)
			s.mu.Lock()
			s.compacting = false
			s.mu.Unlock()
			if err == nil && summary != "" {
				return summary, id
			}
			// LLM 失败：退化截断式（不把失败上抛——压缩尽力而为）
		}
	}
	return truncateSummary(region), id
}

// llmSummary 构建压缩请求并调用聚合 LLM（提示词与宿主原引擎逐字一致）。
func (s *compactionBasicServer) llmSummary(ctx context.Context, llm llmChat, region []*proto.Message) (string, error) {
	prompt := "你是对话压缩器。请将下面的对话历史压缩成一段精简但信息完整的摘要，" +
		"保留关键信息（用户意图、重要决策、工具结果要点），去除冗余细节。只输出摘要，不添加额外解释。\n\n--- 对话历史 ---\n"
	for _, msg := range region {
		prompt += fmt.Sprintf("[%s] %s\n", msg.GetRole(), msg.GetContent())
	}
	compactMsgs := []*proto.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "请压缩上述对话历史。"},
	}
	maxTokens := s.window / 8
	if maxTokens < 512 {
		maxTokens = 512
	}
	resp, err := llm.Chat(ctx, compactMsgs, int32(maxTokens))
	if err != nil {
		return "", fmt.Errorf("compaction-basic LLM call failed: %w", err)
	}
	if resp.GetContent() == "" {
		return "", fmt.Errorf("compaction-basic returned empty summary")
	}
	return resp.GetContent(), nil
}

// pressureThreshold 压力触发阈值（token 数）。
func (s *compactionBasicServer) pressureThreshold() int {
	return int(float64(s.window) * s.threshold)
}

// retainBudget 保留尾部预算（token 数）：Ratio 与 Min 取大（对齐宿主引擎）。
func (s *compactionBasicServer) retainBudget() int {
	b := int(float64(s.window) * s.retainRatio)
	if b < s.retainMin {
		b = s.retainMin
	}
	return b
}

// retainBoundary 保留尾部边界：从末尾向前累积，返回前段可压缩的上界
// （[retainIdx, len) 为保留区）。全部落在保留区时返回 len(msgs)。
func (s *compactionBasicServer) retainBoundary(msgs []*proto.Message) int {
	budget := s.retainBudget()
	acc := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		acc += s.msgTokens(msgs[i])
		if acc > budget {
			return i + 1
		}
	}
	return len(msgs)
}

// msgTokens 字节级启发式 token 估算（对齐宿主 pre-step 估算与原引擎：约 4 字节/token，
// 工具调用结构开销 +8）。
func (s *compactionBasicServer) msgTokens(m *proto.Message) int {
	n := len(m.GetContent()) / 4
	if len(m.GetToolCalls()) > 0 {
		n += 8
	}
	return n
}

// estimateMsgs 估算消息列表总 token 数。
func (s *compactionBasicServer) estimateMsgs(msgs []*proto.Message) int {
	total := 0
	for _, m := range msgs {
		total += s.msgTokens(m)
	}
	return total
}

// truncateSummary 截断式退化摘要：取首条+末条拼接（对齐宿主引擎 truncateCompact，
// 溢出安全：单侧内容截断到 2000 字符，防截断摘要本身仍超预算）。
func truncateSummary(region []*proto.Message) string {
	if len(region) == 0 {
		return ""
	}
	summary := "[压缩摘要] " + capText(region[0].GetContent())
	if len(region) > 1 {
		summary += "\n...\n" + capText(region[len(region)-1].GetContent())
	}
	return summary
}

// truncateCap 截断式摘要单侧内容上限（字符）。
const truncateCap = 2000

// capText 按字符安全截断（防 CJK 拆字）。
func capText(s string) string {
	r := []rune(s)
	if len(r) > truncateCap {
		return string(r[:truncateCap]) + "…[截断]"
	}
	return s
}

// applyRecords 把已压缩记录应用到消息列表：前缀替换为逐条摘要（user 角色，
// 显式标注 compaction-basic 标识），其余原样保留。记录失配/越界时原样返回。
func applyRecords(msgs []*proto.Message, records []compactionRecord) []*proto.Message {
	if len(records) == 0 {
		return msgs
	}
	last := records[len(records)-1]
	if last.UpTo <= 0 || last.UpTo > len(msgs) {
		return msgs
	}
	out := make([]*proto.Message, 0, len(records)+len(msgs)-last.UpTo)
	for _, rec := range records {
		marker := "[上下文压缩摘要 compaction-basic=" + rec.ID
		if rec.Emergency {
			marker += " emergency"
		}
		marker += "]"
		out = append(out, &proto.Message{Role: "user", Content: marker + "\n" + rec.Summary})
	}
	out = append(out, msgs[last.UpTo:]...)
	return out
}

// validRecords 校验既有记录与当前消息列表的指纹吻合性：记录连续覆盖 [0, last.UpTo)，
// 仅最后一条的前缀指纹需要复核。失配（历史被改写/回滚/换会话）时全部作废。
func validRecords(msgs []*proto.Message, records []compactionRecord) []compactionRecord {
	if len(records) == 0 {
		return nil
	}
	last := records[len(records)-1]
	if last.UpTo > 0 && last.UpTo <= len(msgs) && fingerprint(msgs[:last.UpTo]) == last.HeadHash {
		return records
	}
	return nil
}

// fingerprint 前缀指纹：role + NUL + content + NUL 串联的 sha256 hex。
func fingerprint(msgs []*proto.Message) string {
	h := sha256.New()
	for _, m := range msgs {
		h.Write([]byte(m.GetRole()))
		h.Write([]byte{0})
		h.Write([]byte(m.GetContent()))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// rewriteJSON 组装 pre-step 改写结果（宿主 applyPreStepHook 协议：
// {"messages": [...]}，proto.Message 数组 JSON）。
func rewriteJSON(msgs []*proto.Message) (string, error) {
	b, err := json.Marshal(msgs)
	if err != nil {
		return "", err
	}
	return `{"messages": ` + string(b) + `}`, nil
}

// stateFor per-session 压缩记录（进程内 map，须持 s.mu；缺失即空状态）。
// 基础兜底零文件 IO（对齐 DSH compaction-basic：持久化由宿主会话存储承担）；
// 插件重启即重新评估——指纹失配语义的自然延伸，尽力而为。
func (s *compactionBasicServer) stateFor(sessID string) *compactionState {
	st, ok := s.states[sessID]
	if !ok {
		st = &compactionState{}
		s.states[sessID] = st
	}
	return st
}

// sessionKey 会话状态键：优先事件透传的 session_id（宿主 ChatRequest.session_id），
// 缺省按工作区派生项目级键（对齐宿主 session 存储的 SessionKeyForProject——
// 同一项目同名、不同项目隔离）。
func (s *compactionBasicServer) sessionKey(sessID string) string {
	if sessID == "" {
		return session.SessionKeyForProject(dsc.WorkspaceRoot())
	}
	return sessID
}
