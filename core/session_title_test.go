package core

import (
	"context"
	"testing"
)

// mockLLMForTitle 用于标题测试的 mock LLM。
type mockLLMForTitle struct {
	response string
	err      error
}

func (m *mockLLMForTitle) Chat(ctx context.Context, messages []Message, tools []Tool, maxTokens int) (*ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &ChatResponse{Content: m.response}, nil
}

func (m *mockLLMForTitle) ChatStream(ctx context.Context, messages []Message, tools []Tool) (<-chan *ChatStreamResponse, error) {
	return nil, nil
}

func (m *mockLLMForTitle) Name(ctx context.Context) string    { return "mock" }
func (m *mockLLMForTitle) Version(ctx context.Context) string { return "1.0.0" }
func (m *mockLLMForTitle) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockLLMForTitle) VisionEnabled() bool { return false }

func TestSessionTitleGenerate(t *testing.T) {
	titler := NewLLMSessionTitler(&mockLLMForTitle{response: "用户登录接口实现"})

	title, err := titler.Generate(context.Background(), "帮我实现一个用户登录的接口")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if title != "用户登录接口实现" {
		t.Errorf("title = %q, want '用户登录接口实现'", title)
	}
}

func TestSessionTitleFallback(t *testing.T) {
	titler := NewLLMSessionTitler(nil) // 无 LLM

	title, err := titler.Generate(context.Background(), "帮我写一个复杂的数据分析报告")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if title == "" {
		t.Error("fallback title should not be empty")
	}
}

func TestSessionTitleEmpty(t *testing.T) {
	titler := NewLLMSessionTitler(&mockLLMForTitle{response: "新会话"})

	title, err := titler.Generate(context.Background(), "")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if title != "新会话" {
		t.Errorf("title for empty msg = %q, want '新会话'", title)
	}
}

func TestSessionTitleTruncation(t *testing.T) {
	longTitle := "这是一个非常非常非常非常非常非常非常非常非常非常非常非常非常非常非常非常非常长的标题"
	titler := NewLLMSessionTitler(&mockLLMForTitle{response: longTitle})

	title, err := titler.Generate(context.Background(), "测试")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	runes := []rune(title)
	if len(runes) > 50 {
		t.Errorf("title should be truncated to 50 runes, got %d", len(runes))
	}
}
