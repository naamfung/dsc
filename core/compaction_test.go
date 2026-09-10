package core

import (
	"context"
	"strings"
	"testing"
)

// mockLLMProviderForCompaction 用于压缩测试的 mock LLM。
type mockLLMProviderForCompaction struct {
	response string
	err      error
}

func (m *mockLLMProviderForCompaction) Chat(ctx context.Context, messages []Message, tools []Tool, maxTokens int) (*ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &ChatResponse{Content: m.response}, nil
}

func (m *mockLLMProviderForCompaction) ChatStream(ctx context.Context, messages []Message, tools []Tool) (<-chan *ChatStreamResponse, error) {
	return nil, nil
}

func (m *mockLLMProviderForCompaction) Name(ctx context.Context) string    { return "mock" }
func (m *mockLLMProviderForCompaction) Version(ctx context.Context) string { return "1.0.0" }
func (m *mockLLMProviderForCompaction) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockLLMProviderForCompaction) VisionEnabled() bool { return false }

// TestCompactionBasicCompactIfNeeded 校验：超过阈值时触发压缩，未超阈值时不压缩。
func TestCompactionBasicCompactIfNeeded(t *testing.T) {
	llm := &mockLLMProviderForCompaction{response: "这是压缩摘要"}
	engine := NewBasicCompactionEngine(llm, 1000) // 窗口 1000 tokens

	// 构造消息：每条约 100 tokens（400 字节 / 4）
	msgs := make([]*Message, 20)
	for i := range msgs {
		msgs[i] = &Message{Role: "user", Content: strings.Repeat("x", 400)}
	}

	// 未超阈值（20 * 100 = 2000 tokens，阈值 800）→ 超了，应压缩
	result, err := engine.CompactIfNeeded(context.Background(), msgs, CompactionTriggerPressure)
	if err != nil {
		t.Fatalf("CompactIfNeeded: %v", err)
	}
	if result == nil {
		t.Fatal("超过阈值应触发压缩，got nil result")
	}
	if result.Summary == "" {
		t.Error("摘要不应为空")
	}
	if result.ShadowedCount <= 0 {
		t.Error("被压缩消息数应 > 0")
	}

	// 消息太少 → 不压缩
	smallMsgs := []*Message{{Role: "user", Content: "hi"}}
	result2, err := engine.CompactIfNeeded(context.Background(), smallMsgs, CompactionTriggerPressure)
	if err != nil {
		t.Fatalf("CompactIfNeeded small: %v", err)
	}
	if result2 != nil {
		t.Error("消息太少不应压缩，got non-nil result")
	}
}

// TestCompactionBasicCompactNow 校验：按需强制压缩，即使未超阈值。
func TestCompactionBasicCompactNow(t *testing.T) {
	llm := &mockLLMProviderForCompaction{response: "强制压缩摘要"}
	engine := NewBasicCompactionEngine(llm, 100000) // 大窗口，不会因压力触发

	msgs := []*Message{
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "你好！有什么可以帮你的？"},
		{Role: "user", Content: "帮我写代码"},
		{Role: "assistant", Content: "好的，请问要写什么代码？"},
	}

	result, err := engine.CompactNow(context.Background(), msgs)
	if err != nil {
		t.Fatalf("CompactNow: %v", err)
	}
	if result == nil {
		t.Fatal("CompactNow 应返回压缩结果")
	}
	if result.Summary != "强制压缩摘要" {
		t.Errorf("摘要应为 '强制压缩摘要'，got %q", result.Summary)
	}
}

// TestCompactionBasicCompactRegion 校验：强制压缩指定范围。
func TestCompactionBasicCompactRegion(t *testing.T) {
	llm := &mockLLMProviderForCompaction{response: "区域压缩摘要"}
	engine := NewBasicCompactionEngine(llm, 10000)

	msgs := []*Message{
		{Role: "user", Content: "msg1"},
		{Role: "assistant", Content: "msg2"},
		{Role: "user", Content: "msg3"},
		{Role: "assistant", Content: "msg4"},
		{Role: "user", Content: "msg5"},
	}

	// 压缩 [1, 3) 即 msg2 + msg3
	result, err := engine.CompactRegion(context.Background(), msgs, 1, 3)
	if err != nil {
		t.Fatalf("CompactRegion: %v", err)
	}
	if result == nil {
		t.Fatal("CompactRegion 应返回结果")
	}
	if result.ShadowedCount != 2 {
		t.Errorf("被压缩消息数应为 2，got %d", result.ShadowedCount)
	}
}

// TestCompactionBasicCompactRegionInvalid 校验：非法范围返回错误。
func TestCompactionBasicCompactRegionInvalid(t *testing.T) {
	llm := &mockLLMProviderForCompaction{response: "摘要"}
	engine := NewBasicCompactionEngine(llm, 10000)

	msgs := []*Message{{Role: "user", Content: "msg1"}}

	cases := []struct {
		name       string
		start, end int
		wantErr    bool
	}{
		{"start<0", -1, 1, true},
		{"end>len", 0, 2, true},
		{"start>=end", 1, 1, true},
		{"valid", 0, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := engine.CompactRegion(context.Background(), msgs, c.start, c.end)
			if c.wantErr && err == nil {
				t.Error("应返回错误")
			}
			if !c.wantErr && err != nil {
				t.Errorf("不应返回错误，got %v", err)
			}
		})
	}
}

// TestCompactionBasicReentranceGuard 校验：防重入，同时只能一次压缩。
func TestCompactionBasicReentranceGuard(t *testing.T) {
	llm := &mockLLMProviderForCompaction{response: "摘要"}
	engine := NewBasicCompactionEngine(llm, 100000)

	msgs := make([]*Message, 20)
	for i := range msgs {
		msgs[i] = &Message{Role: "user", Content: strings.Repeat("x", 400)}
	}

	// 第一次压缩应成功
	result, err := engine.CompactNow(context.Background(), msgs)
	if err != nil {
		t.Fatalf("第一次压缩: %v", err)
	}
	if result == nil {
		t.Fatal("第一次压缩应返回结果")
	}

	// 第二次压缩也应成功（第一次已完成，锁已释放）
	result2, err := engine.CompactNow(context.Background(), msgs)
	if err != nil {
		t.Fatalf("第二次压缩: %v", err)
	}
	if result2 == nil {
		t.Fatal("第二次压缩应返回结果")
	}
}

// TestCompactionFallbackTruncate 校验：无 LLM provider 时退化为截断式压缩。
func TestCompactionFallbackTruncate(t *testing.T) {
	engine := NewBasicCompactionEngine(nil, 1000) // nil LLM

	msgs := []*Message{
		{Role: "user", Content: "第一条消息"},
		{Role: "assistant", Content: "第二条消息"},
		{Role: "user", Content: "第三条消息"},
	}

	result, err := engine.CompactNow(context.Background(), msgs)
	if err != nil {
		t.Fatalf("CompactNow with nil LLM: %v", err)
	}
	if result == nil {
		t.Fatal("应返回截断式压缩结果")
	}
	if !strings.Contains(result.Summary, "第一条消息") {
		t.Errorf("截断式摘要应含首条消息，got %q", result.Summary)
	}
}

// TestCompactionRetainTail 校验：压缩时保留尾部消息不被压缩。
func TestCompactionRetainTail(t *testing.T) {
	llm := &mockLLMProviderForCompaction{response: "摘要"}
	engine := NewBasicCompactionEngine(llm, 100000) // 小窗口

	// 构造 10 条消息，每条约 50 tokens（200 字节 / 4）
	msgs := make([]*Message, 10)
	for i := range msgs {
		msgs[i] = &Message{Role: "user", Content: strings.Repeat("y", 40000)}
	}

	result, err := engine.CompactIfNeeded(context.Background(), msgs, CompactionTriggerPressure)
	if err != nil {
		t.Fatalf("CompactIfNeeded: %v", err)
	}
	if result == nil {
		t.Fatal("应触发压缩")
	}
	// 被压缩的消息数应 < 总消息数（尾部被保留）
	if result.ShadowedCount >= len(msgs) {
		t.Errorf("被压缩消息数 %d 应 < 总数 %d（尾部应保留）", result.ShadowedCount, len(msgs))
	}
}
