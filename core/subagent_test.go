package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

// scriptedLLM 按脚本逐步返回响应的 LLM provider（测试子代理循环）。
type scriptedLLM struct {
	mu            sync.Mutex
	steps         []*ChatResponse
	chatCalls     int
	toolsCaptured []string // 记录最近一次调用收到的工具名
}

func (p *scriptedLLM) Chat(_ context.Context, _ []Message, tools []Tool, _ int) (*ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.captureToolsLocked(tools)
	if p.chatCalls >= len(p.steps) {
		return nil, errors.New("no more scripted steps")
	}
	r := p.steps[p.chatCalls]
	p.chatCalls++
	return r, nil
}

// captureToolsLocked 记录本次调用收到的工具名（需已持有 p.mu）。
func (p *scriptedLLM) captureToolsLocked(tools []Tool) {
	p.toolsCaptured = make([]string, 0, len(tools))
	for _, t := range tools {
		p.toolsCaptured = append(p.toolsCaptured, t.Name)
	}
}

func (p *scriptedLLM) ChatStream(_ context.Context, _ []Message, tools []Tool) (<-chan *ChatStreamResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.captureToolsLocked(tools)
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

func TestSubagentPassesTools(t *testing.T) {
	m := newSubagentManager()
	llm := &scriptedLLM{steps: []*ChatResponse{
		{ToolCalls: []ToolCall{{ID: "c1", Name: "plain-tool", Arguments: map[string]any{}}}},
		{Content: "done", FinishReason: "stop"},
	}}
	m.llms["p"] = llm
	m.llmOrder = []string{"p"}
	m.agentLLMName = "p"

	if _, err := m.RunSubagent(context.Background(), &SubagentRequest{Prompt: "do it"}); err != nil {
		t.Fatalf("RunSubagent: %v", err)
	}
	// 子代理的 LLM 请求必须携带宿主聚合工具目录（否则无法真正发出工具调用）
	llm.mu.Lock()
	defer llm.mu.Unlock()
	found := false
	for _, n := range llm.toolsCaptured {
		if n == "plain-tool" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("subagent llm received tools %v, want plain-tool included", llm.toolsCaptured)
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

// ctxAwareLLM 感知 ctx 的 LLM：ctx 已取消时 ChatStream 返回 ctx 错误，
// 用于验证子代理的 LLM 请求正确透传调用方上下文（取消失效会表现为挂起）。
type ctxAwareLLM struct {
	gotCancel bool
}

func (p *ctxAwareLLM) Chat(_ context.Context, _ []Message, _ []Tool, _ int) (*ChatResponse, error) {
	return &ChatResponse{Content: "done", FinishReason: "stop"}, nil
}

func (p *ctxAwareLLM) ChatStream(ctx context.Context, _ []Message, _ []Tool) (<-chan *ChatStreamResponse, error) {
	if ctx.Err() != nil {
		p.gotCancel = true
		return nil, ctx.Err()
	}
	ch := make(chan *ChatStreamResponse, 1)
	ch <- &ChatStreamResponse{Content: "done", FinishReason: "stop"}
	close(ch)
	return ch, nil
}

func (p *ctxAwareLLM) Name(context.Context) string    { return "ctx-aware" }
func (p *ctxAwareLLM) Version(context.Context) string { return "1.0.0" }
func (p *ctxAwareLLM) HealthCheck(context.Context) error {
	return nil
}

func TestSubagentPropagatesCancelContext(t *testing.T) {
	m := newSubagentManager()
	llm := &ctxAwareLLM{}
	m.llms["p"] = llm
	m.llmOrder = []string{"p"}
	m.agentLLMName = "p"

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 调用前即取消，模拟主 agent 被中断
	_, err := m.RunSubagent(ctx, &SubagentRequest{Prompt: "do it"})
	if err == nil {
		t.Fatal("RunSubagent with cancelled ctx should fail")
	}
	if !llm.gotCancel {
		t.Fatal("subagent llm call should receive the cancelled ctx (context not propagated)")
	}
}

// TestSubagentDefaultIterations 校验子代理默认（未传 max_iterations）可完成多轮工具任务：
// 默认无迭代上限，退出由模型/进度决定（返回纯文本即完成）。
func TestSubagentDefaultIterations(t *testing.T) {
	m := newSubagentManager()
	// 三步：工具调用 ×2 + 最终结果
	llm := &scriptedLLM{steps: []*ChatResponse{
		{ToolCalls: []ToolCall{{ID: "c1", Name: "plain-tool", Arguments: map[string]any{}}}},
		{ToolCalls: []ToolCall{{ID: "c2", Name: "plain-tool", Arguments: map[string]any{}}}},
		{Content: "final", FinishReason: "stop"},
	}}
	m.llms["p"] = llm
	m.llmOrder = []string{"p"}
	m.agentLLMName = "p"

	res, err := m.ExecuteTool(context.Background(), "subagent", json.RawMessage(`{"prompt":"do it"}`))
	if err != nil {
		t.Fatalf("subagent with default iterations should complete: %v", err)
	}
	if res != "final" {
		t.Fatalf("result = %q, want final", res)
	}
}

// TestSubagentUnlimitedByDefault 回归：默认（未传 max_iterations）不应因超过旧的 8 轮
// 上限而失败——只要模型持续调用工具推进，长程任务可继续，直至模型返回纯文本收尾。
func TestSubagentUnlimitedByDefault(t *testing.T) {
	m := newSubagentManager()
	steps := make([]*ChatResponse, 0, 11)
	// 10 轮连续工具调用（远超旧默认上限 8），最后以纯文本收尾
	for i := 0; i < 10; i++ {
		steps = append(steps, &ChatResponse{ToolCalls: []ToolCall{{ID: "c", Name: "plain-tool", Arguments: map[string]any{}}}})
	}
	steps = append(steps, &ChatResponse{Content: "long task done", FinishReason: "stop"})
	llm := &scriptedLLM{steps: steps}
	m.llms["p"] = llm
	m.llmOrder = []string{"p"}
	m.agentLLMName = "p"

	res, err := m.RunSubagent(context.Background(), &SubagentRequest{Prompt: "long task"})
	if err != nil {
		t.Fatalf("RunSubagent with unlimited default should complete all rounds: %v", err)
	}
	if res != "long task done" {
		t.Fatalf("result = %q, want 'long task done'", res)
	}
	// 11 步全部被执行（10 轮工具 + 1 轮收尾），证明未被 8 轮上限截断
	if llm.chatCalls != 11 {
		t.Fatalf("llm calls = %d, want 11 (must not be capped at former default 8)", llm.chatCalls)
	}
}

// TestSubagentToolHonorsMaxIterations 校验工具参数 max_iterations 透传到运行循环。
func TestSubagentToolHonorsMaxIterations(t *testing.T) {
	m := newSubagentManager()
	llm := &scriptedLLM{steps: []*ChatResponse{
		{ToolCalls: []ToolCall{{ID: "c1", Name: "plain-tool", Arguments: map[string]any{}}}},
		{Content: "final", FinishReason: "stop"},
	}}
	m.llms["p"] = llm
	m.llmOrder = []string{"p"}
	m.agentLLMName = "p"

	// max_iterations=1：第一轮工具调用后即达上限报错，证明参数被透传
	_, err := m.ExecuteTool(context.Background(), "subagent", json.RawMessage(`{"prompt":"do it","max_iterations":1}`))
	if err == nil || !strings.Contains(err.Error(), "exceeded 1 iterations") {
		t.Fatalf("err = %v, want iteration limit from max_iterations", err)
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
