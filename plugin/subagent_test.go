package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

// scriptedLLM 按脚本逐步返回响应的 LLM provider（测试子代理循环）。
type scriptedLLM struct {
	mu        sync.Mutex
	steps     []*ChatResponse
	chatCalls int
}

func (p *scriptedLLM) Chat(context.Context, []Message, []Tool, int) (*ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.chatCalls >= len(p.steps) {
		return nil, errors.New("no more scripted steps")
	}
	r := p.steps[p.chatCalls]
	p.chatCalls++
	return r, nil
}

func (p *scriptedLLM) ChatStream(_ context.Context, _ []Message, _ []Tool) (<-chan *ChatStreamResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.chatCalls >= len(p.steps) {
		return nil, errors.New("no more scripted steps")
	}
	r := p.steps[p.chatCalls]
	p.chatCalls++
	ch := make(chan *ChatStreamResponse, 1)
	ch <- &ChatStreamResponse{
		Content:      r.Content,
		FinishReason: r.FinishReason,
		ToolCalls:    r.ToolCalls,
	}
	close(ch)
	return ch, nil
}

func (p *scriptedLLM) Name(context.Context) string    { return "scripted" }
func (p *scriptedLLM) Version(context.Context) string { return "1.0.0" }
func (p *scriptedLLM) HealthCheck(context.Context) error {
	return nil
}

func newSubagentManager() *Manager {
	m := newRouterManager() // 替换事件总线去掉默认 retry
	_ = m.toolRegistry.Register(&mockTool{name: "plain-tool"})
	return m
}

func TestSubagentToolLoop(t *testing.T) {
	m := newSubagentManager()
	llm := &scriptedLLM{steps: []*ChatResponse{
		{ToolCalls: []ToolCall{{ID: "c1", Name: "plain-tool", Arguments: map[string]any{}}}},
		{Content: "final answer", FinishReason: "stop"},
	}}
	m.llms["p"] = llm
	m.llmOrder = []string{"p"}
	m.agentLLMName = "p"

	result, err := m.ExecuteTool(context.Background(), "subagent", json.RawMessage(`{"prompt":"do the task"}`))
	if err != nil {
		t.Fatalf("subagent: %v", err)
	}
	if result != "final answer" {
		t.Fatalf("result = %q, want final answer", result)
	}
	// 两步：工具调用 + 最终结果
	if llm.chatCalls != 2 {
		t.Fatalf("llm calls = %d, want 2", llm.chatCalls)
	}
}

func TestSubagentIterationLimit(t *testing.T) {
	m := newSubagentManager()
	llm := &scriptedLLM{steps: []*ChatResponse{
		{ToolCalls: []ToolCall{{ID: "c1", Name: "plain-tool", Arguments: map[string]any{}}}},
		{ToolCalls: []ToolCall{{ID: "c2", Name: "plain-tool", Arguments: map[string]any{}}}},
		{ToolCalls: []ToolCall{{ID: "c3", Name: "plain-tool", Arguments: map[string]any{}}}},
	}}
	m.llms["p"] = llm
	m.llmOrder = []string{"p"}
	m.agentLLMName = "p"

	_, err := m.RunSubagent(context.Background(), &SubagentRequest{Prompt: "loop", MaxIterations: 2})
	if err == nil || err.Error() != "subagent exceeded 2 iterations" {
		t.Fatalf("err = %v, want iteration limit", err)
	}
}

func TestSubagentToolRegistered(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	if _, ok := m.toolRegistry.Get("subagent"); !ok {
		t.Fatal("subagent tool should be registered by default")
	}
}

func TestSubagentToolRequiresPrompt(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	_, err := m.ExecuteTool(context.Background(), "subagent", json.RawMessage(`{}`))
	if err == nil || err.Error() != "subagent: prompt is required" {
		t.Fatalf("err = %v, want missing prompt error", err)
	}
}
