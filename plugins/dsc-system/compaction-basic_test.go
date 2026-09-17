package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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
func bigMsgs(n, charsPer int) []*proto.Message {
	msgs := make([]*proto.Message, 0, n)
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs = append(msgs, &proto.Message{
			Role:    role,
			Content: strings.Repeat("x", charsPer),
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
	s := newTestCompactionBasicServer(t, 10000) // 阈值 8000；保留预算 max(1600,1024)=1600
	fake := &fakeCompactionBasicLLM{content: "LLM-SUMMARY"}
	s.llm = fake

	msgs := bigMsgs(16, 6000) // 每条 1500 token，共 24000
	res := mustPreStep(t, s, msgs)
	if res == "" {
		t.Fatalf("over-threshold pre-step must rewrite")
	}
	got := parseRewrite(t, res)
	// 保留尾部：预算 1600，单条 1500 → 只留最后 1 条（1500≤1600、3000>1600 →
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
	msgs := bigMsgs(16, 6000)
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
	msgs := bigMsgs(16, 6000)
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
	msgs := bigMsgs(16, 6000)
	res := mustPreStep(t, s, msgs)
	if res == "" {
		t.Fatalf("failure must degrade to truncate rewrite")
	}
	if got := parseRewrite(t, res); !strings.Contains(got[0].GetContent(), "[压缩摘要]") {
		t.Fatalf("must fall back to truncate summary")
	}
}

// TestCompactionBasicEmergencyRetainLast 溢出紧急压缩：保留最后 1 条、截断式摘要、
// 返回 {"retry": true}；非溢出错误码不触发。
func TestCompactionBasicEmergencyRetainLast(t *testing.T) {
	s := newTestCompactionBasicServer(t, 1000) // 阈值 800：估算 1800 ≥ 阈值
	s.retainMin = 2000                         // 保留预算盖过全部消息 → pre-step 不动作（全部落保留区）
	msgs := bigMsgs(6, 1200)                   // 每条 300 token，共 1800
	// 第一次 pre-step：估算超阈值但保留区覆盖全部 → 不改写
	if res := mustPreStep(t, s, msgs); res != "" {
		t.Fatalf("all-in-retain pre-step must not rewrite, got %q", res)
	}
	// 紧急：压缩 [0,5)（保留最后 1 条）
	res, err := s.handleHostEvent(context.Background(), "agent/request-error",
		`{"agent":"a","code":"context_window_exceeded"}`)
	if err != nil || res != `{"retry": true}` {
		t.Fatalf("emergency must request retry, res %q err %v", res, err)
	}
	// 重试的 pre-step：指纹命中 → 应用改写
	res2 := mustPreStep(t, s, msgs)
	got := parseRewrite(t, res2)
	if len(got) != 2 {
		t.Fatalf("after emergency rewrite = %d messages, want 2", len(got))
	}
	if !strings.Contains(got[0].GetContent(), "emergency") {
		t.Fatalf("emergency record must be marked, got %q", got[0].GetContent())
	}
	if got[1].GetContent() != msgs[5].GetContent() {
		t.Fatalf("last message must be preserved")
	}
	// 非溢出错误码：不触发
	res3, err := s.handleHostEvent(context.Background(), "agent/request-error",
		`{"agent":"a","code":"rate_limited"}`)
	if err != nil || res3 != "" {
		t.Fatalf("non-overflow error must be ignored, got %q", res3)
	}
}

// TestCompactionBasicSmallHistoryNoOp 消息太少/用量低：零开销不改写。
func TestCompactionBasicSmallHistoryNoOp(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000)
	msgs := bigMsgs(3, 400) // 300 token << 8000
	if res := mustPreStep(t, s, msgs); res != "" {
		t.Fatalf("small history must not rewrite, got %q", res)
	}
}
