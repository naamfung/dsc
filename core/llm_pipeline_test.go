package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"dsc/proto"
	"google.golang.org/grpc/metadata"
)

// mockLLMProvider 测试用 LLM provider：可配置前 N 次失败。
type mockLLMProvider struct {
	mu        sync.Mutex
	failChat  int  // Chat 剩余失败次数
	failStart int  // ChatStream 建立阶段剩余失败次数
	midErr    bool // 流中途失败（已发帧后）
	chatCalls int
}

func (p *mockLLMProvider) Chat(context.Context, []Message, []Tool, int) (*ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.chatCalls++
	if p.failChat > 0 {
		p.failChat--
		return nil, errors.New("provider unavailable")
	}
	return &ChatResponse{Content: "ok", FinishReason: "stop"}, nil
}

func (p *mockLLMProvider) ChatStream(ctx context.Context, _ []Message, _ []Tool) (<-chan *ChatStreamResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failStart > 0 {
		p.failStart--
		return nil, errors.New("stream start failed")
	}
	ch := make(chan *ChatStreamResponse, 2)
	ch <- &ChatStreamResponse{Content: "hi"}
	if p.midErr {
		ch <- &ChatStreamResponse{Error: "mid stream error"}
	}
	close(ch)
	return ch, nil
}

func (p *mockLLMProvider) Name(context.Context) string    { return "mock-llm" }
func (p *mockLLMProvider) Version(context.Context) string { return "1.0.0" }
func (p *mockLLMProvider) VisionEnabled() bool            { return true }
func (p *mockLLMProvider) HealthCheck(context.Context) error {
	return nil
}

// mockChatStream 实现 gRPC 流式发送端。
type mockChatStream struct {
	sent []*proto.ChatStreamResponse
}

func (s *mockChatStream) Send(r *proto.ChatStreamResponse) error {
	s.sent = append(s.sent, r)
	return nil
}

func (s *mockChatStream) Context() context.Context     { return context.Background() }
func (s *mockChatStream) RecvMsg(any) error            { return nil }
func (s *mockChatStream) SendMsg(any) error            { return nil }
func (s *mockChatStream) SetHeader(metadata.MD) error  { return nil }
func (s *mockChatStream) SendHeader(metadata.MD) error { return nil }
func (s *mockChatStream) SetTrailer(metadata.MD)       {}

func TestLLMChatRetrySucceeds(t *testing.T) {
	p := &mockLLMProvider{failChat: 1}
	b := NewEventBus()
	b.OnWaterfall(EventLLMRequest, LLMRetryListener(3, time.Millisecond))
	srv := &llmProviderServer{provider: p, events: b}

	resp, err := srv.Chat(context.Background(), &proto.ChatRequest{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if p.chatCalls != 2 {
		t.Fatalf("chat calls = %d, want 2 (1 fail + 1 retry)", p.chatCalls)
	}
}

func TestLLMChatRetryExhausted(t *testing.T) {
	p := &mockLLMProvider{failChat: 5}
	b := NewEventBus()
	b.OnWaterfall(EventLLMRequest, LLMRetryListener(2, time.Millisecond))
	srv := &llmProviderServer{provider: p, events: b}

	_, err := srv.Chat(context.Background(), &proto.ChatRequest{})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if p.chatCalls != 2 {
		t.Fatalf("chat calls = %d, want 2 (maxAttempts)", p.chatCalls)
	}
}

func TestLLMRequestVeto(t *testing.T) {
	p := &mockLLMProvider{}
	b := NewEventBus()
	b.OnWaterfall(EventLLMRequest, func(ctx EventContext, next func(EventContext) error) error {
		return errors.New("vetoed by policy")
	})
	srv := &llmProviderServer{provider: p, events: b}

	_, err := srv.Chat(context.Background(), &proto.ChatRequest{})
	if err == nil || err.Error() != "vetoed by policy" {
		t.Fatalf("err = %v, want vetoed by policy", err)
	}
	if p.chatCalls != 0 {
		t.Fatalf("provider should not be called on veto, got %d", p.chatCalls)
	}
}

func TestLLMChatStreamRetryOnStartFailure(t *testing.T) {
	p := &mockLLMProvider{failStart: 1}
	b := NewEventBus()
	b.OnWaterfall(EventLLMRequest, LLMRetryListener(3, time.Millisecond))
	srv := &llmProviderServer{provider: p, events: b}
	stream := &mockChatStream{}

	err := srv.ChatStream(&proto.ChatRequest{}, stream)
	if err != nil {
		t.Fatalf("chat stream: %v", err)
	}
	if len(stream.sent) != 1 || stream.sent[0].Content != "hi" {
		t.Fatalf("sent = %+v, want one 'hi' frame", stream.sent)
	}
}

func TestLLMChatStreamNoRetryAfterFrames(t *testing.T) {
	p := &mockLLMProvider{midErr: true}
	b := NewEventBus()
	b.OnWaterfall(EventLLMRequest, LLMRetryListener(3, time.Millisecond))
	srv := &llmProviderServer{provider: p, events: b}
	stream := &mockChatStream{}

	err := srv.ChatStream(&proto.ChatRequest{}, stream)
	if err == nil || err.Error() != "LLM stream error: mid stream error" {
		t.Fatalf("err = %v, want mid stream error", err)
	}
	// 已发 1 帧后失败：不得重试（避免重复输出），sent 只有 1 帧
	if len(stream.sent) != 1 {
		t.Fatalf("sent %d frames, want 1 (no retry after frames)", len(stream.sent))
	}
}
