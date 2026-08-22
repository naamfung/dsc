package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"dsc/proto"
	"google.golang.org/grpc/metadata"
)

// Subagent 子代理（对齐 DSH subagent 的 in-process provider）：宿主侧轻量
// agent 执行器，接受委派的 prompt，驱动「LLM 调用 → 工具执行」小循环直至
// 模型不再请求工具，返回最终结果。LLM 走多 provider 聚合路由（含瀑布/重试），
// 工具走执行流水线（含策略拦截），与主 agent 共享同一套宿主机制。
// 对外以 subagent 工具暴露，agent 可通过工具调用委派子任务。

// SubagentRequest 子代理请求。
type SubagentRequest struct {
	Prompt        string
	MaxIterations int // 模型-工具循环轮数上限；<=0 时使用默认值
}

const defaultSubagentIterations = 3

// RunSubagent 执行一次子代理任务：system 引导 + prompt 进入循环，
// 每轮调用聚合 LLM 服务；有工具调用则逐个经工具流水线执行并把结果回填，
// 直至模型返回纯文本（或达到迭代上限）。
func (m *Manager) RunSubagent(ctx context.Context, req *SubagentRequest) (string, error) {
	if req.MaxIterations <= 0 {
		req.MaxIterations = defaultSubagentIterations
	}
	// 子代理同样拿到宿主聚合工具目录（互通：cron/subagent 任务可调用工具插件，
	// 与主 agent 从聚合 ToolService.ListTools 拿到的一致）
	tools := m.AllToolsProto()
	msgs := []*proto.Message{
		{Role: "system", Content: "You are a subagent executing a delegated task. " +
			"Complete the task using the available tools if needed, then return only the final result, concise."},
		{Role: "user", Content: req.Prompt},
	}
	agg := &llmAggregateServer{m: m}
	for i := 0; i < req.MaxIterations; i++ {
		// 走流式聚合（与主 agent 一致）：unary Chat 在 thinking 模式下可能只返回
		// thinking 块而 text 为空，流式帧则完整携带文本增量
		col := &frameCollector{}
		if err := agg.ChatStream(&proto.ChatRequest{Messages: msgs, Tools: tools}, col); err != nil {
			return "", fmt.Errorf("subagent llm call: %w", err)
		}
		var content string
		var toolCalls []*proto.ToolCall
		for _, f := range col.frames {
			content += f.Content
			if len(f.ToolCalls) > 0 {
				toolCalls = f.ToolCalls
			}
		}
		assistantMsg := &proto.Message{Role: "assistant", Content: content}
		if len(toolCalls) > 0 {
			assistantMsg.ToolCalls = toolCalls
		}
		msgs = append(msgs, assistantMsg)
		if len(toolCalls) == 0 {
			return content, nil
		}
		for idx, tc := range toolCalls {
			callID := tc.Id
			if callID == "" {
				callID = fmt.Sprintf("subagent_%d", idx) // 部分 provider 不返回 id，补齐以关联结果
			}
			tc.Id = callID
			result, err := m.ExecuteTool(ctx, tc.Name, json.RawMessage(tc.ArgumentsJson))
			content := result
			if err != nil {
				content = "Error executing tool: " + err.Error()
			}
			msgs = append(msgs, &proto.Message{Role: "tool", Content: content, ToolCallId: callID})
		}
	}
	return "", fmt.Errorf("subagent exceeded %d iterations", req.MaxIterations)
}

// frameCollector 内部收集流式帧，供子代理循环以流式路径调用聚合 LLM 服务。
type frameCollector struct {
	frames []*proto.ChatStreamResponse
}

func (c *frameCollector) Send(r *proto.ChatStreamResponse) error {
	c.frames = append(c.frames, r)
	return nil
}

func (c *frameCollector) Context() context.Context     { return context.Background() }
func (c *frameCollector) RecvMsg(any) error            { return nil }
func (c *frameCollector) SendMsg(any) error            { return nil }
func (c *frameCollector) SetHeader(metadata.MD) error  { return nil }
func (c *frameCollector) SendHeader(metadata.MD) error { return nil }
func (c *frameCollector) SetTrailer(metadata.MD)       {}

// subagentTool 暴露 subagent 工具给主 agent 的模型调用。
type subagentTool struct{ m *Manager }

func (t *subagentTool) Name() string { return "subagent" }

func (t *subagentTool) Description() string {
	return "Spawn a subagent to execute a delegated task (a self-contained prompt run) " +
		"and return its final result. Use for tasks you can delegate and summarize."
}

func (t *subagentTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {"type": "string", "description": "The task to delegate to the subagent."}
		},
		"required": ["prompt"]
	}`)
}

func (t *subagentTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("subagent: invalid args: %w", err)
	}
	if p.Prompt == "" {
		return "", fmt.Errorf("subagent: prompt is required")
	}
	return t.m.RunSubagent(ctx, &SubagentRequest{Prompt: p.Prompt})
}
