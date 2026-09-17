package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

// TestApplyPreStepHookCarriesLastUsageTokens 回归「纯启发式低估导致压缩不触发」：
// 宿主按会话记录最近一次成功请求的上报 prompt 用量，并在 agent/pre-step 事件透传
// （last_usage_tokens）；压缩插件以该精确底数判定压力，而非仅靠字节启发式估算。
// 覆盖跨层字段保真：Manager 记录 → llmAggregateServer.applyPreStepHook → 插件事件 JSON。
func TestApplyPreStepHookCarriesLastUsageTokens(t *testing.T) {
	m := newRouterManager()
	m.recordSessionUsage("sess-x", 90000)

	f := &fakeHook{events: make(chan *proto.OnEventRequest, 1)}
	m.toolHookClients["p1"] = f
	m.toolHookOrder = []string{"p1"}

	srv := &llmAggregateServer{m: m}
	req := &proto.ChatRequest{
		SessionId: "sess-x",
		Messages:  []*proto.Message{{Role: "user", Content: "hi"}},
	}
	out := srv.applyPreStepHook(context.Background(), req)
	if out == nil || len(out.Messages) != 1 || out.Messages[0].Content != "hi" {
		t.Fatalf("无改写插件时请求必须原样通过, got %+v", out)
	}
	select {
	case ev := <-f.events:
		if ev.GetName() != string(EventAgentPreStep) {
			t.Fatalf("event name = %s", ev.GetName())
		}
		var payload AgentPreStepEvent
		if err := json.Unmarshal([]byte(ev.GetDataJson()), &payload); err != nil {
			t.Fatalf("parse event: %v", err)
		}
		if payload.LastUsageTokens != 90000 {
			t.Fatalf("last_usage_tokens = %d, want 90000", payload.LastUsageTokens)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pre-step event not dispatched to hook")
	}
}

// TestRecordSessionUsageRequiresPositive 空会话 id / 非正用量不记录（内部调用与非流式请求
// 不污染会话状态），lastSessionPrompt 返回 0 由插件退回启发式。
func TestRecordSessionUsageRequiresPositive(t *testing.T) {
	m := newRouterManager()
	m.recordSessionUsage("", 90000)
	m.recordSessionUsage("sess-y", 0)
	if got := m.lastSessionPrompt("sess-y"); got != 0 {
		t.Fatalf("lastSessionPrompt = %d, want 0", got)
	}
}
