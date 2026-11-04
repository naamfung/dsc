package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	dsc "dsc-sdk"

	"dsc/proto"
)

// fakeCompactionBasicLLM 聚合 LLM 的 fake（llmChat 接口）：计数调用、返回固定摘要。
type fakeCompactionBasicLLM struct {
	calls   int
	content string
	err     error
}

func (f *fakeCompactionBasicLLM) Chat(ctx context.Context, messages []*proto.Message, maxTokens int32) (*proto.ChatResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &proto.ChatResponse{Content: f.content}, nil
}

// newTestCompactionBasicServer 构建受控驻留（绕开 env：直接设字段）。
func newTestCompactionBasicServer(t *testing.T, window int) *compactionBasicServer {
	t.Helper()
	s := newCompactionBasicServer()
	s.window = window
	return s
}

// bigMsgs 生成 n 条约 tokensPer 条的大消息（user/assistant 交替）。
//
// 实现按 dsc.EstimateTextTokens 的口径构造：ASCII 约 4 字节/token，故每个 token 写 4 个
// 字节。此前实现直接写 charsPer 个字节——那时估算把每个字符当 1 token，两者恰好相等；
// 估算改成「按字符分类累加」后（修掉纯 ASCII 高估 4~5 倍的 bug），这里必须同步换算，
// 否则夹具只有预期压力的 1/4，压缩阈值再也够不到。
func bigMsgs(n, tokensPer int) []*proto.Message {
	msgs := make([]*proto.Message, 0, n)
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs = append(msgs, &proto.Message{
			Role:    role,
			Content: strings.Repeat("x", tokensPer*4),
		})
	}
	return msgs
}

func mustPreStep(t *testing.T, s *compactionBasicServer, msgs []*proto.Message) string {
	t.Helper()
	return mustPreStepSession(t, s, "sess-c1", msgs)
}

func mustPreStepSession(t *testing.T, s *compactionBasicServer, sess string, msgs []*proto.Message) string {
	t.Helper()
	msgsJSON, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("marshal msgs: %v", err)
	}
	evJSON, err := json.Marshal(map[string]any{
		"agent": "agent-react-loop", "session": sess, "messages_json": string(msgsJSON),
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	res, err := s.handleHostEvent(context.Background(), "agent/pre-step", string(evJSON))
	if err != nil {
		t.Fatalf("pre-step: %v", err)
	}
	return res
}

// mustPreStepUsage 携带 last_usage_tokens（宿主透传的上次服务端上报用量）的 pre-step。
// mustPreStepProjected 构造 pre-step 事件并带上宿主给的投影压力。
// 本插件只认 projected_tokens（压力合成在宿主，见 core.projectPressure）；
// last_usage_tokens 仍照旧带上，用于确认插件**没有**再自作主张用它。
func mustPreStepProjected(t *testing.T, s *compactionBasicServer, sess string, msgs []*proto.Message, projected, lastUsage int) string {
	t.Helper()
	msgsJSON, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("marshal msgs: %v", err)
	}
	evJSON, err := json.Marshal(map[string]any{
		"agent": "agent-react-loop", "session": sess, "messages_json": string(msgsJSON),
		"projected_tokens": projected, "last_usage_tokens": lastUsage,
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	res, err := s.handleHostEvent(context.Background(), "agent/pre-step", string(evJSON))
	if err != nil {
		t.Fatalf("pre-step: %v", err)
	}
	return res
}

func mustPreStepUsage(t *testing.T, s *compactionBasicServer, sess string, msgs []*proto.Message, lastUsage int) string {
	t.Helper()
	return mustPreStepProjected(t, s, sess, msgs, lastUsage, lastUsage)
}

// parseRewrite 解析 {"messages": [...]} 改写结果（非空且含至少一条消息）。
func parseRewrite(t *testing.T, res string) []*proto.Message {
	t.Helper()
	var m struct {
		Messages []*proto.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(res), &m); err != nil {
		t.Fatalf("parse rewrite %q: %v", res, err)
	}
	if len(m.Messages) == 0 {
		t.Fatalf("rewrite has no messages: %q", res)
	}
	return m.Messages
}

// TestCompactionBasicDisabledByZeroWindow 驻留禁用（窗口 0）：一切事件静止。
func TestCompactionBasicDisabledByZeroWindow(t *testing.T) {
	s := newTestCompactionBasicServer(t, 0)
	msgs := bigMsgs(6, 1200)
	if res := mustPreStep(t, s, msgs); res != "" {
		t.Fatalf("disabled resident must not rewrite, got %q", res)
	}
	res, err := s.handleHostEvent(context.Background(), "agent/request-error",
		`{"agent":"a","code":"context_window_exceeded"}`)
	if err != nil || res != "" {
		t.Fatalf("disabled resident must ignore request-error, got %q err %v", res, err)
	}
}

// TestCompactionBasicPressureLLMSummary 压力触发：超阈值时保留尾部、前段经 LLM 摘要、
// 返回 {"messages":[...]}；同载荷重放复用状态（LLM 不再被调用）。
func TestCompactionBasicPressureLLMSummary(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000) // 阈值 7000；保留预算 max(1600,1024)=1600
	fake := &fakeCompactionBasicLLM{content: "LLM-SUMMARY"}
	s.llm = fake

	msgs := bigMsgs(16, 1200) // 每条 1204 token（CJK 感知估算），共 19264
	res := mustPreStep(t, s, msgs)
	if res == "" {
		t.Fatalf("over-threshold pre-step must rewrite")
	}
	got := parseRewrite(t, res)
	// 保留尾部：预算 1600，单条 1204 → 只留最后 1 条（1204≤1600、2408>1600 →
	// retainIdx=15）；改写 = 1 摘要 + 1 条尾段
	if len(got) != 2 {
		t.Fatalf("rewrite = %d messages, want 2 (summary + tail)", len(got))
	}
	if !strings.Contains(got[0].GetContent(), "LLM-SUMMARY") {
		t.Fatalf("head must be LLM summary, got %q", got[0].GetContent())
	}
	if got[1].GetContent() != msgs[15].GetContent() {
		t.Fatalf("tail message must be preserved verbatim")
	}
	if fake.calls != 1 {
		t.Fatalf("LLM must be called once, got %d", fake.calls)
	}

	// 同载荷重放：指纹命中，状态复用，不再调 LLM
	res2 := mustPreStep(t, s, msgs)
	got2 := parseRewrite(t, res2)
	if len(got2) != 2 || got2[0].GetContent() != got[0].GetContent() {
		t.Fatalf("replay must reuse state deterministically")
	}
	if fake.calls != 1 {
		t.Fatalf("replay must not re-summarize, LLM calls = %d", fake.calls)
	}
}

// TestCompactionBasicFingerprintMismatchResets 历史被改写（指纹失配）：记录作废、
// 从头重新评估并重新摘要。
func TestCompactionBasicFingerprintMismatchResets(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000)
	fake := &fakeCompactionBasicLLM{content: "S1"}
	s.llm = fake
	msgs := bigMsgs(16, 1200)
	if res := mustPreStep(t, s, msgs); res == "" {
		t.Fatalf("must rewrite")
	}
	// 改写历史头部 → 指纹失配
	msgs[0].Content = "rewritten-history"
	fake.content = "S2"
	res := mustPreStep(t, s, msgs)
	got := parseRewrite(t, res)
	if !strings.Contains(got[0].GetContent(), "S2") {
		t.Fatalf("stale records must be discarded and re-summarized, got %q", got[0].GetContent())
	}
	if fake.calls != 2 {
		t.Fatalf("LLM calls after reset = %d, want 2", fake.calls)
	}
}

// TestCompactionBasicTruncateFallback LLM 未互联：截断式退化摘要（首条+末条拼接）。
func TestCompactionBasicTruncateFallback(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000) // llm 为 nil
	msgs := bigMsgs(16, 1200)
	res := mustPreStep(t, s, msgs)
	got := parseRewrite(t, res)
	if len(got) != 2 {
		t.Fatalf("rewrite = %d, want 2", len(got))
	}
	if !strings.Contains(got[0].GetContent(), "[压缩摘要]") {
		t.Fatalf("nil-LLM path must produce truncate summary, got %q", got[0].GetContent())
	}
}

// TestCompactionBasicLLMFailureDegrades LLM 调用失败：退化截断式，不把失败上抛。
func TestCompactionBasicLLMFailureDegrades(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000)
	s.llm = &fakeCompactionBasicLLM{err: errors.New("provider down")}
	msgs := bigMsgs(16, 1200)
	res := mustPreStep(t, s, msgs)
	if res == "" {
		t.Fatalf("failure must degrade to truncate rewrite")
	}
	if got := parseRewrite(t, res); !strings.Contains(got[0].GetContent(), "[压缩摘要]") {
		t.Fatalf("must fall back to truncate summary")
	}
}

// TestCompactionBasicIgnoresOverflowEvent 超窗的处置权归宿主（收紧预算 + 滑窗 + 重试），
// 压缩驻留对 agent/request-error 一律不响应。
//
// 曾经这里会做「紧急压缩 → 返回 retry」：跳过 LLM、用截断摘要把一整段真实消息换成一句话，
// 且落盘成块永久生效——比滑窗（只影响本次请求）严格更差。何况一次只压一个区间，不保证
// 收敛（实测三次重试仍超窗）。
func TestCompactionBasicIgnoresOverflowEvent(t *testing.T) {
	s := newTestCompactionBasicServer(t, 1000)
	s.retainMin = 10000
	msgs := bigMsgs(6, 1200)
	if res := mustPreStep(t, s, msgs); res != "" {
		t.Fatalf("all-in-retain pre-step must not rewrite, got %q", res)
	}
	res, err := s.handleHostEvent(context.Background(), "agent/request-error",
		`{"agent":"a","code":"context_window_exceeded"}`)
	if err != nil || res != "" {
		t.Fatalf("插件不应处理超窗事件，res %q err %v", res, err)
	}
	// 状态没被改动：再来一次 pre-step 仍然不改写
	if res := mustPreStep(t, s, msgs); res != "" {
		t.Fatalf("不应因 request-error 产生压缩记录，got %q", res)
	}
}

// TestCompactionBasicSmallHistoryNoOp 消息太少/用量低：零开销不改写。
func TestCompactionBasicSmallHistoryNoOp(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000)
	msgs := bigMsgs(3, 400) // 300 token << 7000
	if res := mustPreStep(t, s, msgs); res != "" {
		t.Fatalf("small history must not rewrite, got %q", res)
	}
}

// TestCompactionBasicTriggersOnReportedUsage 回归「纯启发式低估导致压缩不触发」：
// 字节启发式估算（不含工具定义、CJK 每字 3 字节被 /4 低估）把真实用量显著估低
// （实测真实 83.56% 而插件估算仅 60.08%），但宿主已透传上次服务端上报用量
// （last_usage_tokens）超阈值时，必须触发压缩改写。
func TestCompactionBasicTriggersOnReportedUsage(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000) // 阈值 7000
	s.llm = &fakeCompactionBasicLLM{content: "LLM-SUMMARY"}

	msgs := bigMsgs(8, 1200) // 每条 1204 token（CJK 感知估算），共 9632
	res := mustPreStepUsage(t, s, "sess-u1", msgs, 9000)
	if res == "" {
		t.Fatalf("上报用量 9000 >= 阈值 7000 时必须触发压缩")
	}
	got := parseRewrite(t, res)
	if len(got) >= len(msgs) || !strings.Contains(got[0].GetContent(), "LLM-SUMMARY") {
		t.Fatalf("rewrite 未落地：%d msgs, head=%q", len(got), got[0].GetContent())
	}
}

// TestCompactionBasicMeasuresItsOwnSurface 压力必须按「本插件要发出的表面」算：
//   - 尚无压缩记录时，其表面 == 请求表面 → 采信宿主投影（含工具目录与信封判据）；
//   - 已有压缩记录时，表面已被改写（摘要 + 保留尾部），宿主投影仍按原始列表算，
//     若照用会把已压到 ~2 条的视图当成 900k → 每步再摘要一次（回归：把 80% 阈值
//     退化成「保留区一满就压」，实测会多打一次 LLM）。
func TestCompactionBasicMeasuresItsOwnSurface(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000) // 阈值 7000
	fake := &fakeCompactionBasicLLM{content: "LLM-SUMMARY"}
	s.llm = fake

	// ① 无记录：宿主投影 9000 ≥ 阈值 → 压缩（哪怕本地估算只有 ~4.8k）
	small := bigMsgs(4, 1200)
	res := mustPreStepProjected(t, s, "sess-own-1", small, 9000, 9000)
	if res == "" {
		t.Fatal("宿主投影 9000 ≥ 阈值时必须压缩")
	}
	if fake.calls != 1 {
		t.Fatalf("应只摘要一次，got %d", fake.calls)
	}

	// ② 无记录：宿主投影 6500 < 阈值 7000 → 不压缩（插件不得用本地估算越权决策）
	big := bigMsgs(8, 1200)
	if res := mustPreStepProjected(t, s, "sess-own-2", big, 6500, 6500); res != "" {
		t.Fatalf("宿主投影低于阈值时不得压缩，got %q", res)
	}

	// ③ 有记录：尾部只略增（渲染后远低于阈值），宿主投影仍按原始 19k 给 →
	// 允许返回既有改写（隐藏已压段），但**不得新增摘要**。
	msgs := bigMsgs(16, 1200)
	if mustPreStepProjected(t, s, "sess-own-3", msgs, 19000, 19000) == "" {
		t.Fatal("首次应压缩")
	}
	before := fake.calls
	tailPlus2 := append(append([]*proto.Message{}, msgs...), bigMsgs(2, 1200)...)
	if res := mustPreStepProjected(t, s, "sess-own-3", tailPlus2, 21000, 21000); res == "" {
		t.Fatal("有记录时应返回既有改写（否则模型看到未压缩的全量历史）")
	}
	if fake.calls != before {
		t.Fatalf("渲染后表面 ~2.4k << 阈值 7000，不得再摘要；摘要次数 %d → %d", before, fake.calls)
	}

	// ④ 尾部长到渲染后表面超阈值 → 允许再摘要
	tailPlus8 := append(append([]*proto.Message{}, msgs...), bigMsgs(8, 1200)...)
	mustPreStepProjected(t, s, "sess-own-3", tailPlus8, 27000, 27000)
	if fake.calls <= before {
		t.Fatalf("渲染后表面超过阈值时应再次摘要，摘要次数仍为 %d", fake.calls)
	}
}

// TestCompactionWindowFromContextWindowEnv 窗口来源优先级回归：宿主把有效窗口
// （配置 context_window / 端点探测值）以 DSC_CONTEXT_WINDOW 注入所有插件，压缩器
// 必须采信——此前只认自己的专属键，云端 1M 窗口仍按 128K 判压力，几步就压缩。
func TestCompactionWindowFromContextWindowEnv(t *testing.T) {
	t.Setenv("DSC_COMPACTION_BASIC_CONTEXT_WINDOW", "")
	t.Setenv("DSC_CONTEXT_WINDOW", "1048576")
	if s := newCompactionBasicServer(); s.window != 1048576 {
		t.Fatalf("应采信 DSC_CONTEXT_WINDOW=1048576，got %d", s.window)
	}

	// 插件专属键优先于通用键
	t.Setenv("DSC_COMPACTION_BASIC_CONTEXT_WINDOW", "262144")
	if s := newCompactionBasicServer(); s.window != 262144 {
		t.Fatalf("专属键应优先，got %d", s.window)
	}

	// 两者都缺席 → 128K 默认（本地兜底不变）
	t.Setenv("DSC_COMPACTION_BASIC_CONTEXT_WINDOW", "")
	t.Setenv("DSC_CONTEXT_WINDOW", "")
	if s := newCompactionBasicServer(); s.window != 131072 {
		t.Fatalf("无配置时默认 131072，got %d", s.window)
	}

	// 专属键显式 0 = 驻留禁用（不得被通用键顶回）
	t.Setenv("DSC_CONTEXT_WINDOW", "1048576")
	t.Setenv("DSC_COMPACTION_BASIC_CONTEXT_WINDOW", "0")
	if s := newCompactionBasicServer(); s.window != 0 {
		t.Fatalf("显式 0 应禁用，got %d", s.window)
	}
}

// TestRetainBoundaryKeepsToolPairing 保留区起点不得落在 assistant(tool_calls) 与其
// tool_result 之间。
//
// 预算边界是纯按 token 累积出来的，落在中间就会把调用压进摘要、结果留在保留区——
// provider 收到孤立的 tool 消息直接 400，整轮失败。契约检查工具
// （tools/backendcheck，按 compaction 能力扫全部插件）在真实派生历史上抓到过这个
// 切点，这里用同一形状的历史把边界钉住：任何预算下保留区首条都不能是 tool。
func TestRetainBoundaryKeepsToolPairing(t *testing.T) {
	msgs := []*proto.Message{{Role: "user", Content: strings.Repeat("x", 400)}}
	for i := 0; i < 5; i++ {
		callID := "c" + string(rune('0'+i))
		msgs = append(msgs,
			&proto.Message{Role: "assistant", ToolCalls: []*proto.ToolCall{{Id: callID, Name: "shell", ArgumentsJson: "{}"}}},
			&proto.Message{Role: "tool", ToolCallId: callID, Content: strings.Repeat("y", 400)},
		)
	}

	s := newCompactionBasicServer()
	s.window = 0 // 预算只由 retainMin 决定，便于逐档扫
	for budget := 1; budget <= 3000; budget++ {
		s.retainMin = budget
		idx := s.retainBoundary(msgs)
		if idx < len(msgs) && msgs[idx].GetRole() == "tool" {
			t.Fatalf("预算 %d 时保留区首条是孤立 tool 结果（下标 %d，callID=%s）；provider 会 400",
				budget, idx, msgs[idx].GetToolCallId())
		}
		if idx < 1 || idx > len(msgs) {
			t.Fatalf("预算 %d 时边界越界: %d", budget, idx)
		}
	}
}

// nilLLMClient 是一个「接口不为 nil、指针为 nil」的客户端——正是
// (*llmclient.Client)(nil) 装进 llmChat 接口后的样子。
type nilLLMClient struct{ called bool }

func (n *nilLLMClient) Chat(context.Context, []*proto.Message, int32) (*proto.ChatResponse, error) {
	n.called = true
	panic("nil client must never be called")
}

// TestCompactionBasicTypedNilLLMDoesNotPanic 宿主在 agent 注册前完成互通握手时，
// ic.LLM() 返回的是 (*llmclient.Client)(nil)。它装进接口后 `c == nil` 为 false，旧实现
// 直接存下，下一次 summarize 就 nil deref——**插件进程 panic 退出**，而会话日志上看起来
// 只是「到达阈值却没有压缩」（实测：331 步的单轮里窗口涨到 93%，最后滑窗兜不住而报错）。
//
// 真实地址无法在单测里构造，这里用同样的形状（接口非 nil、指针 nil）复现。
func TestCompactionBasicTypedNilLLMDoesNotPanic(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000)
	var typedNil *nilLLMClient
	s.attachLLM(typedNil) // 接口里装着 nil 指针
	if s.llm != nil {
		t.Fatal("typed nil 不得被存为可用客户端")
	}

	msgs := bigMsgs(16, 1200)
	res := mustPreStep(t, s, msgs) // 走到阈值：不得 panic
	if res == "" {
		t.Fatal("压力已过阈值：应返回改写结果")
	}
	if got := parseRewrite(t, res); !strings.Contains(got[0].GetContent(), "[压缩摘要]") {
		t.Fatalf("无可用 LLM 时应退化为截断式摘要，got %q", got[0].GetContent())
	}
}

// TestCompactionBasicAttachRealLLMStillWorks 正常的客户端仍然要被接受（别把判空做成永久关闭）。
func TestCompactionBasicAttachRealLLMStillWorks(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000)
	fake := &fakeCompactionBasicLLM{content: "摘要正文"}
	s.attachLLM(fake)

	msgs := bigMsgs(16, 1200)
	res := mustPreStep(t, s, msgs)
	if res == "" {
		t.Fatal("应返回改写结果")
	}
	if fake.calls == 0 {
		t.Fatal("注入的 LLM 未被调用")
	}
	if got := parseRewrite(t, res); !strings.Contains(got[0].GetContent(), "摘要正文") {
		t.Fatalf("应使用 LLM 摘要，got %q", got[0].GetContent())
	}
}

// sizeRecordingLLM 记录每次摘要请求的规模（按 SDK 统一估算），用于验证分块边界。
type sizeRecordingLLM struct {
	calls   int
	prompts []int
}

func (r *sizeRecordingLLM) Chat(_ context.Context, msgs []*proto.Message, maxTokens int32) (*proto.ChatResponse, error) {
	r.calls++
	r.prompts = append(r.prompts, dsc.EstimateProtoMessagesTokens(msgs))
	if maxTokens > summaryMaxTokens {
		panic("摘要输出预算超出上限")
	}
	return &proto.ChatResponse{Content: "S"}, nil
}

// ctxAwareLLM 按 ctx 取消的 fake：验证「预算耗尽就停在已完成的块上」。
type ctxAwareLLM struct {
	calls int
	delay time.Duration
}

func (c *ctxAwareLLM) Chat(ctx context.Context, _ []*proto.Message, _ int32) (*proto.ChatResponse, error) {
	c.calls++
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(c.delay):
		return &proto.ChatResponse{Content: "S"}, nil
	}
}

// TestCompactionBasicDefaultThreshold 默认阈值 0.70：宿主的收口预算是窗口的 80%
// （估算偏差的兜底余量），压缩阈值必须留在它之下才留得出余量——贴得太近时，
// 一次估算偏差或一轮的增量（工具结果动辄几万 token）就能把「压缩」直接推成
// 「撞收口后整轮失败」。
func TestCompactionBasicDefaultThreshold(t *testing.T) {
	t.Setenv("DSC_COMPACTION_BASIC_THRESHOLD", "")
	t.Setenv("DSC_COMPACTION_BASIC_CONTEXT_WINDOW", "")
	t.Setenv("DSC_CONTEXT_WINDOW", "")
	s := newCompactionBasicServer()
	if s.threshold != 0.70 {
		t.Fatalf("默认阈值应为 0.70，got %v", s.threshold)
	}
	if s.threshold >= 0.80 {
		t.Fatalf("阈值 %v 没给宿主的 80%% 收口留出余量", s.threshold)
	}
	// 显式覆盖仍然有效（用户自己承担与收口预算贴太近的后果）
	t.Setenv("DSC_COMPACTION_BASIC_THRESHOLD", "0.55")
	if got := newCompactionBasicServer().threshold; got != 0.55 {
		t.Fatalf("显式阈值应生效，got %v", got)
	}
}

// TestCompactionBasicChunksLargeRegion 大区间必须分块摘要：整段塞进一条 prompt 时，
// 请求本身会顶到窗口收口预算、又跑不完宿主 30s 的分发预算（现场：摘要超时 → 宿主的
// 改写被丢弃 → 全量历史照旧发出 → 撞收口整轮失败；同时摘要退化成截断式，15 万 token
// 换一句话永久落盘）。
//
// 分块后每块都在预算内，且每条记录连续覆盖前缀（多块 = 多段摘要）。
func TestCompactionBasicChunksLargeRegion(t *testing.T) {
	s := newTestCompactionBasicServer(t, 200000) // 阈值 140000；保留 32000
	fake := &sizeRecordingLLM{}
	s.llm = fake
	msgs := bigMsgs(60, 4000) // 每条 ~4004 token，共 ~240k

	res := mustPreStepSession(t, s, "sess-chunk", msgs)
	if res == "" {
		t.Fatal("超阈值应改写")
	}
	if fake.calls < 2 {
		t.Fatalf("20 万 token 的区间必须分块摘要，实际只调了 %d 次", fake.calls)
	}
	st := s.states["sess-chunk"]
	if len(st.Records) != fake.calls {
		t.Fatalf("记录数应与摘要次数一致：%d != %d", len(st.Records), fake.calls)
	}
	for i, p := range fake.prompts {
		if p > summaryChunkTokens+5000 {
			t.Fatalf("第 %d 块 prompt 过大（%d token）：分块失效", i, p)
		}
	}
	// 覆盖连续、且止于保留边界：记录首尾相接（中间不许有缝），否则 applyRecords 会丢掉一段历史
	prev := 0
	for i, r := range st.Records {
		if i == 0 {
			if r.UpTo-r.Covered < 0 {
				t.Fatalf("首条记录起点非法: %+v", r)
			}
			prev = r.Covered
			continue
		}
		if r.UpTo-r.Covered != prev {
			t.Fatalf("第 %d 块与上一块不连续：起点 %d，上块止于 %d", i, r.UpTo-r.Covered, prev)
		}
		prev = r.UpTo
	}
	last := st.Records[len(st.Records)-1]
	if last.UpTo >= len(msgs) {
		t.Fatalf("压缩不得越过保留区：up_to=%d，共 %d 条", last.UpTo, len(msgs))
	}
	if got := parseRewrite(t, res); len(got) != len(st.Records)+len(msgs)-last.UpTo {
		t.Fatalf("改写应为 %d 段摘要 + %d 条尾段，实际 %d 条", len(st.Records), len(msgs)-last.UpTo, len(got))
	}
}

// TestCompactionBasicStopsOnDeadlineWithoutTruncating 摘要预算耗尽时必须停在
// 已完成的块上，**不得**退化成截断式摘要：那会把没摘要过的历史换成一句话永久落盘
// （现场 5 条记录全是 source=truncated，每段 15 万 token 的历史被压成 2 千字符）。
func TestCompactionBasicStopsOnDeadlineWithoutTruncating(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000)
	s.llmBudget = 20 * time.Millisecond
	fake := &ctxAwareLLM{delay: 5 * time.Second}
	s.llm = fake

	msgs := bigMsgs(16, 1200)
	start := time.Now()
	res := mustPreStepSession(t, s, "sess-deadline", msgs)
	if res != "" {
		t.Fatalf("一块都没压成时不得返回改写，got %q", res)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("必须按预算提前返回（否则宿主 30s 分发超时会把改写丢掉），耗时 %v", elapsed)
	}
	if st := s.states["sess-deadline"]; len(st.Records) != 0 {
		t.Fatalf("超时不得落任何记录（尤其不能是截断式摘要）：%+v", st.Records)
	}
}
