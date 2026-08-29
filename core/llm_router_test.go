package core

import (
	"context"
	"testing"

	"dsc/proto"
)

// newRouterManager 构造路由测试用的 Manager：替换事件总线去掉默认 retry，
// 使 provider 失败即触发 fallback（不被重试掩盖）。
func newRouterManager() *Manager {
	m := NewManager(&ManagerConfig{})
	m.events = NewEventBus()
	return m
}

func TestAggregateChatFallsBack(t *testing.T) {
	m := newRouterManager()
	primary := &mockLLMProvider{failChat: 1}
	backup := &mockLLMProvider{}
	m.llms["primary"] = primary
	m.llms["backup"] = backup
	m.llmOrder = []string{"primary", "backup"}
	m.agentLLMName = "primary"

	srv := &llmAggregateServer{m: m}
	resp, err := srv.Chat(context.Background(), &proto.ChatRequest{})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if primary.chatCalls != 1 || backup.chatCalls != 1 {
		t.Fatalf("calls = primary:%d backup:%d, want 1/1 (fallback after primary failure)", primary.chatCalls, backup.chatCalls)
	}
}

func TestAggregateChatPrimaryFirst(t *testing.T) {
	m := newRouterManager()
	primary := &mockLLMProvider{}
	backup := &mockLLMProvider{}
	m.llms["primary"] = primary
	m.llms["backup"] = backup
	m.llmOrder = []string{"primary", "backup"}
	m.agentLLMName = "primary"

	srv := &llmAggregateServer{m: m}
	if _, err := srv.Chat(context.Background(), &proto.ChatRequest{}); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if primary.chatCalls != 1 || backup.chatCalls != 0 {
		t.Fatalf("calls = primary:%d backup:%d, want 1/0 (backup unused)", primary.chatCalls, backup.chatCalls)
	}
}

func TestAggregateChatAllFail(t *testing.T) {
	m := newRouterManager()
	m.llms["a"] = &mockLLMProvider{failChat: 5}
	m.llms["b"] = &mockLLMProvider{failChat: 5}
	m.llmOrder = []string{"a", "b"}
	m.agentLLMName = "a"

	srv := &llmAggregateServer{m: m}
	if _, err := srv.Chat(context.Background(), &proto.ChatRequest{}); err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestAggregateChatStreamFallbackOnStartFailure(t *testing.T) {
	m := newRouterManager()
	primary := &mockLLMProvider{failStart: 1}
	backup := &mockLLMProvider{}
	m.llms["primary"] = primary
	m.llms["backup"] = backup
	m.llmOrder = []string{"primary", "backup"}
	m.agentLLMName = "primary"

	srv := &llmAggregateServer{m: m}
	stream := &mockChatStream{}
	if err := srv.ChatStream(&proto.ChatRequest{}, stream); err != nil {
		t.Fatalf("chat stream: %v", err)
	}
	if len(stream.sent) != 1 || stream.sent[0].Content != "hi" {
		t.Fatalf("sent = %+v, want one 'hi' frame", stream.sent)
	}
}

func TestAggregateChatStreamNoFallbackAfterFrames(t *testing.T) {
	m := newRouterManager()
	primary := &mockLLMProvider{midErr: true}
	backup := &mockLLMProvider{}
	m.llms["primary"] = primary
	m.llms["backup"] = backup
	m.llmOrder = []string{"primary", "backup"}
	m.agentLLMName = "primary"

	srv := &llmAggregateServer{m: m}
	stream := &mockChatStream{}
	err := srv.ChatStream(&proto.ChatRequest{}, stream)
	if err == nil || err.Error() != "LLM stream error: mid stream error" {
		t.Fatalf("err = %v, want mid stream error", err)
	}
	// 已发帧后失败：不切换 provider，backup 不调用
	if backup.chatCalls != 0 {
		t.Fatalf("backup should not be used after frames, calls = %d", backup.chatCalls)
	}
	if len(stream.sent) != 1 {
		t.Fatalf("sent %d frames, want 1", len(stream.sent))
	}
}

func TestLLMRouteOrderPrimaryFirst(t *testing.T) {
	m := newRouterManager()
	m.llms["primary"] = &mockLLMProvider{}
	m.llms["backup"] = &mockLLMProvider{}
	m.llms["other"] = &mockLLMProvider{}
	m.llmOrder = []string{"primary", "backup", "other"}
	m.agentLLMName = "backup" // primary 声明为 backup

	nap := m.llmRouteSnapshot()
	order := make([]string, 0, len(nap))
	for _, np := range nap {
		order = append(order, np.name)
	}
	want := []string{"backup", "primary", "other"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i, n := range want {
		if order[i] != n {
			t.Fatalf("order[%d] = %s, want %s (order=%v)", i, order[i], n, order)
		}
	}
}

func TestLLMRouteOrderSkipsUnloaded(t *testing.T) {
	m := newRouterManager()
	m.llms["a"] = &mockLLMProvider{}
	m.llmOrder = []string{"a", "ghost"} // ghost 未加载
	m.agentLLMName = ""

	nap := m.llmRouteSnapshot()
	order := make([]string, 0, len(nap))
	for _, np := range nap {
		order = append(order, np.name)
	}
	if len(order) != 1 || order[0] != "a" {
		t.Fatalf("order = %v, want [a] (unloaded skipped)", order)
	}
}
