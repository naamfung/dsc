package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"dsc/core"
	"dsc/proto"
)

// imgRef 生成测试用图像引用（形态合法即可，卸载决策不触达存储）。
func imgRef(n int) string {
	return "dsc-shot://" + strings.Repeat("a", 8) + string(rune('0'+n%10))
}

// TestOffloadRequestImagesBelowBudget 预算内（含边界恰好等于上限）原样返回，
// 零拷贝零改写。
func TestOffloadRequestImagesBelowBudget(t *testing.T) {
	msgs := []*proto.Message{
		{Role: "user", Content: "u", Images: []string{imgRef(1)}},
		{Role: "tool", Content: "t1", Images: []string{imgRef(2), imgRef(3)}},
		{Role: "tool", Content: "t2"},
	}
	got := offloadRequestImages(msgs, 3)
	if len(got) != len(msgs) {
		t.Fatalf("length changed: %d != %d", len(got), len(msgs))
	}
	for i := range msgs {
		if got[i] != msgs[i] {
			t.Fatalf("message %d was copied despite being within budget", i)
		}
	}
}

// TestOffloadRequestImagesOldestFirst 超预算时最旧优先退役：退役的引用从
// Images 移除并在该消息 Content 尾部追加稳定占位文本；最新图像保持在场。
func TestOffloadRequestImagesOldestFirst(t *testing.T) {
	r1, r2, r3, r4 := imgRef(1), imgRef(2), imgRef(3), imgRef(4)
	msgs := []*proto.Message{
		{Role: "user", Content: "u", Images: []string{r1}},
		{Role: "tool", Content: "t1", Images: []string{r2, r3}},
		{Role: "tool", Content: "t2", Images: []string{r4}},
	}
	got := offloadRequestImages(msgs, 2)
	if len(got) != len(msgs) {
		t.Fatalf("length changed: %d != %d", len(got), len(msgs))
	}
	// 最旧两张（r1、r2）退役：user 消息图像清空 + 占位；t1 只留 r3
	if len(got[0].Images) != 0 || !strings.Contains(got[0].Content, "[image omitted to fit request image limits; "+r1+"]") {
		t.Fatalf("oldest image not offloaded: %+v", got[0])
	}
	if len(got[1].Images) != 1 || got[1].Images[0] != r3 {
		t.Fatalf("second oldest not offloaded: %+v", got[1].Images)
	}
	if !strings.Contains(got[1].Content, "[image omitted to fit request image limits; "+r2+"]") {
		t.Fatalf("placeholder missing on partially offloaded message: %q", got[1].Content)
	}
	if len(got[2].Images) != 1 || got[2].Images[0] != r4 {
		t.Fatalf("newest image must stay: %+v", got[2].Images)
	}
}

// TestOffloadRequestImagesTransient 瞬态投影不改传入消息（会话存储不受影响）：
// 消息本体浅拷贝，原消息的 Content/Images 保持原样。
func TestOffloadRequestImagesTransient(t *testing.T) {
	r1, r2 := imgRef(1), imgRef(2)
	msgs := []*proto.Message{
		{Role: "user", Content: "u", Images: []string{r1}},
		{Role: "tool", Content: "t1", Images: []string{r2}},
	}
	_ = offloadRequestImages(msgs, 1)
	if len(msgs[0].Images) != 1 || msgs[0].Images[0] != r1 || msgs[0].Content != "u" {
		t.Fatalf("durable message mutated: %+v", msgs[0])
	}
	if len(msgs[1].Images) != 1 || msgs[1].Content != "t1" {
		t.Fatalf("durable message mutated: %+v", msgs[1])
	}
}

// TestOffloadRequestImagesUnlimited "0" 表示不限制：任何数量原样返回。
func TestOffloadRequestImagesUnlimited(t *testing.T) {
	msgs := []*proto.Message{{Role: "tool", Content: "t", Images: []string{imgRef(1), imgRef(2), imgRef(3)}}}
	if got := offloadRequestImages(msgs, 0); got[0] != msgs[0] {
		t.Fatal("maxImages<=0 must be a no-op (unlimited)")
	}
}

// TestMaxRequestImages 上限配置：env 覆盖、"0"=不限制、非法值回退默认。
func TestMaxRequestImages(t *testing.T) {
	t.Setenv(envMaxRequestImages, "5")
	if got := maxRequestImages(); got != 5 {
		t.Fatalf("maxRequestImages = %d, want 5", got)
	}
	t.Setenv(envMaxRequestImages, "0")
	if got := maxRequestImages(); got != 0 {
		t.Fatalf("maxRequestImages(0) = %d, want 0（不限制）", got)
	}
	t.Setenv(envMaxRequestImages, "bogus")
	if got := maxRequestImages(); got != defaultMaxRequestImages {
		t.Fatalf("非法值应回退默认，got %d", got)
	}
	os.Unsetenv(envMaxRequestImages)
	if got := maxRequestImages(); got != defaultMaxRequestImages {
		t.Fatalf("未设置应取默认，got %d", got)
	}
}

// preStepPayload 构造 agent/pre-step 事件载荷（消息列表 JSON 内嵌）。
func preStepPayload(t *testing.T, msgs []*proto.Message) string {
	t.Helper()
	msgsJSON, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("marshal msgs: %v", err)
	}
	data, err := json.Marshal(map[string]any{
		"agent": "agent-react-loop", "session": "t",
		"messages_json": string(msgsJSON), "token_count": 10,
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return string(data)
}

// TestImageOffloadHandlePreStep 事件面投影：超预算返回 {"messages": [...]}
// （占位 + 最旧清空），预算内返回 ""（零开销）。
func TestImageOffloadHandlePreStep(t *testing.T) {
	s := newImageOffloadServer()
	ctx := context.Background()
	r1, r2, r3 := imgRef(1), imgRef(2), imgRef(3)
	msgs := []*proto.Message{
		{Role: "user", Content: "u", Images: []string{r1}},
		{Role: "tool", Content: "t1", Images: []string{r2, r3}},
	}
	// 预算 3：总量 3 ≤ 3，零开销
	if res, err := s.handleHostEvent(ctx, string(core.EventAgentPreStep), preStepPayload(t, msgs)); err != nil || res != "" {
		t.Fatalf("within budget must be no-op, got %q err=%v", res, err)
	}
	// 预算 1：r1、r2 退役
	t.Setenv(envMaxRequestImages, "1")
	res, err := s.handleHostEvent(ctx, string(core.EventAgentPreStep), preStepPayload(t, msgs))
	if err != nil {
		t.Fatalf("handleHostEvent: %v", err)
	}
	var out struct {
		Messages []*proto.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("parse rewrite: %v", err)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(out.Messages))
	}
	if len(out.Messages[0].Images) != 0 || !strings.Contains(out.Messages[0].Content, "[image omitted to fit request image limits; "+r1+"]") {
		t.Fatalf("oldest not offloaded: %+v", out.Messages[0])
	}
	if len(out.Messages[1].Images) != 1 || out.Messages[1].Images[0] != r3 {
		t.Fatalf("newest must stay: %+v", out.Messages[1].Images)
	}
	// 非预算事件（request-error 等）：忽略
	if res, err := s.handleHostEvent(ctx, string(core.EventAgentRequestError), `{"agent":"a","code":"context_window_exceeded"}`); err != nil || res != "" {
		t.Fatalf("request-error must be ignored, got %q err=%v", res, err)
	}
}

// TestChainPreStep 链式拼接：上游改写结果（{"messages": [...]}）替换载荷中的
// 消息列表，其余字段（agent/session/token_count/user_input）原样保留。
func TestChainPreStep(t *testing.T) {
	orig := []*proto.Message{{Role: "user", Content: "old"}}
	payload := preStepPayload(t, orig)
	rewritten := []*proto.Message{
		{Role: "user", Content: "[压缩摘要] old"},
		{Role: "assistant", Content: "tail", Images: []string{imgRef(9)}},
	}
	rewJSON, err := json.Marshal(map[string]any{"messages": rewritten})
	if err != nil {
		t.Fatalf("marshal rewrite: %v", err)
	}
	chained, err := chainPreStep(payload, string(rewJSON))
	if err != nil {
		t.Fatalf("chainPreStep: %v", err)
	}
	var ev core.AgentPreStepEvent
	if err := json.Unmarshal([]byte(chained), &ev); err != nil {
		t.Fatalf("parse chained payload: %v", err)
	}
	var msgs []*proto.Message
	if err := json.Unmarshal([]byte(ev.MessagesJSON), &msgs); err != nil {
		t.Fatalf("parse chained messages: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Content != "[压缩摘要] old" || len(msgs[1].Images) != 1 {
		t.Fatalf("chained messages mismatch: %+v", msgs)
	}
	if ev.Agent != "agent-react-loop" || ev.Session != "t" || ev.TokenCount != 10 {
		t.Fatalf("payload fields must be preserved: %+v", ev)
	}
}
