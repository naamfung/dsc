package core

import (
	"context"
	"encoding/json"
	"fmt"

	"dsc/proto"
	"google.golang.org/grpc/metadata"
)

// SubagentRequest 子代理请求。
type SubagentRequest struct {
	Prompt        string
	MaxIterations int // 模型-工具循环轮数上限；>0 时生效，<=0 表示无上限（退出由模型/进度决定）
}

// RunSubagent 执行一次子代理任务：system 引导 + prompt 进入循环，
// 每轮调用聚合 LLM 服务；有工具调用则逐个经工具流水线执行并把结果回填，
// 直至模型返回纯文本（自然退出）。不设刚性默认迭代上限——长程任务只要模型持续
// 产出有用的工具调用就会一直推进，何时收尾由模型自行决定（返回纯文本即完成）。
// 需要显式防失控时由调用方传入 max_iterations；外部中断经由 ctx 取消（父 agent
// 被取消时子代理 LLM 调用随之失败，见 TestSubagentPropagatesCancelContext）。
func (m *Manager) RunSubagent(ctx context.Context, req *SubagentRequest) (string, error) {
	// 子代理同样拿到宿主聚合工具目录（互通：cron/subagent 任务可调用工具插件，
	// 与主 agent 从聚合 ToolService.ListTools 拿到的一致）
	tools := m.AllToolsProto()
	msgs := []*proto.Message{
		{Role: "system", Content: "You are a delegated subagent executing a task for a parent agent. " +
			"Your permission scope is the session's file/sandbox policy and cannot be widened from inside this task; " +
			"an operation denied by that policy is rejected automatically. If an operation is denied, do not retry it — " +
			"state the limitation in your final result so the delegating agent can handle it. " +
			"Complete the task using the available tools if needed, then return only the final result, concise. " +
			"You decide when the task is done; there is no iteration limit on you."},
		{Role: "user", Content: req.Prompt},
	}
	agg := &llmAggregateServer{m: m}
	iteration := 0
	for {
		// 显式上限仅在调用方要求时生效（防失控）；默认无上限，退出交由模型/进度决定。
		if req.MaxIterations > 0 && iteration >= req.MaxIterations {
			return "", fmt.Errorf("subagent exceeded %d iterations", req.MaxIterations)
		}
		// 走流式聚合（与主 agent 一致）：unary Chat 在 thinking 模式下可能只返回
		// thinking 块而 text 为空，流式帧则完整携带文本增量
		col := &frameCollector{ctx: ctx}
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
		iteration++
	}
}

// frameCollector 内部收集流式帧，供子代理循环以流式路径调用聚合 LLM 服务。
// ctx 透传调用方（主 agent）的上下文：聚合 LLM 服务的 ChatStream 用
// stream.Context() 作为 provider 请求 ctx，若不透传则取消失效——主 agent 被
// 中断后子代理的 LLM 请求仍会挂起等待流，表现为「卡死/像超时无响应」。
type frameCollector struct {
	ctx    context.Context
	frames []*proto.ChatStreamResponse
}

func (c *frameCollector) Send(r *proto.ChatStreamResponse) error {
	c.frames = append(c.frames, r)
	return nil
}

func (c *frameCollector) Context() context.Context     { return c.ctx }
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
		"and return its final result. Use for tasks you can delegate and summarize. " +
		"The subagent runs until it decides the task is done (no iteration limit by default), " +
		"so it suits long-running work; set max_iterations to impose an explicit cap if needed."
}

func (t *subagentTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {"type": "string", "description": "The task to delegate to the subagent."},
			"max_iterations": {"type": "integer", "description": "Optional hard cap on model-tool rounds; omit for no limit (the subagent stops when it decides the task is done)."}
		},
		"required": ["prompt"]
	}`)
}

func (t *subagentTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Prompt        string `json:"prompt"`
		MaxIterations int    `json:"max_iterations"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("subagent: invalid args: %w", err)
	}
	if p.Prompt == "" {
		return "", fmt.Errorf("subagent: prompt is required")
	}
	return t.m.RunSubagent(ctx, &SubagentRequest{Prompt: p.Prompt, MaxIterations: p.MaxIterations})
}
