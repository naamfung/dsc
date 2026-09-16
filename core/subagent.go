package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
//
// 子代理的工具调用一律按 APPROVAL=never 执行：
//   - 子代理是非交互执行（无 TUI 用户在场等审批），任何 ask 路径都会让循环卡死
//   - 由 DSH 经验：delegated subagent 默认 never，需审批的操作应在主会话里先获授权
//   - 经 WithApprovalPolicy(ctx, "never") 注入 ctx，工具流水线优先采用此策略
//   - 沙箱升级路径（approvalEscalation）见 never 直接拒，与 DSH NEVER 语义一致
//
// 子代理「活跃续命」超时（对齐 shell 的 idle timeout）：
//   - 超时预算与模型可见文案由超时决策插件（timeout-policy）在 tool/execute 槽
//     裁决，宿主工具流水线机械安装执行域（WithIdleDeadline）——调用方 ctx 携带
//     执行域（经 ExecuteTool("subagent") 进入时自动成立）
//   - 循环每收到一帧 LLM 输出或一次工具结果就 TouchActivity 续命：本地慢速模型
//     持续生成时不会被误杀；provider 真挂起（无任何输出）时超时退出而非永久阻塞
//   - ctx 未携带执行域（直接调用、或策略插件未装）时不设超时，TouchActivity
//     为 no-op
func (m *Manager) RunSubagent(ctx context.Context, req *SubagentRequest) (string, error) {
	// 子代理的工具调用一律 never：无 TUI 用户在场审批，ask 会卡死
	// （沙箱升级路径下 never 直接拒，与 DSH NEVER 语义一致）
	toolCtx := WithApprovalPolicy(ctx, "never")

	// 子代理工具目录排除 subagent 自身，防止递归调用导致栈深爆 / 无限递归
	allTools := m.AllToolsProto()
	tools := make([]*proto.Tool, 0, len(allTools))
	for _, t := range allTools {
		if t.Name == "subagent" {
			continue
		}
		tools = append(tools, t)
	}

	msgs := []*proto.Message{
		{Role: "system", Content: subagentSystemPrompt(req.MaxIterations)},
		{Role: "user", Content: req.Prompt},
	}
	agg := &llmAggregateServer{m: m}
	iteration := 0

	var finalResult string
	for {
		// 显式上限仅在调用方要求时生效（防失控）；默认无上限，退出交由模型/进度决定。
		if req.MaxIterations > 0 && iteration >= req.MaxIterations {
			return "", fmt.Errorf("subagent exceeded %d iterations", req.MaxIterations)
		}

		// 走流式聚合（与主 agent 一致）：unary Chat 在 thinking 模式下可能只返回
		// thinking 块而 text 为空，流式帧则完整携带文本增量
		//
		// 关键：流式帧到达时经 TouchActivity(ctx) 续命——只要模型在持续生成，
		// 活跃续命看门狗不会超时；只有 provider 真挂起（无任何输出）才会超时取消。
		col := &frameCollector{ctx: ctx} // 每帧到达都 TouchActivity
		chatErr := agg.ChatStream(&proto.ChatRequest{Messages: msgs, Tools: tools}, col)
		TouchActivity(ctx) // LLM 调用结束也算活动（无论成功失败）
		if chatErr != nil {
			if ctx.Err() != nil {
				// ctx 被取消（含空闲看门狗触发）：返回取消错误，
				// 文案由执行域安装方（工具流水线桥按裁决）统一替换
				return "", ctx.Err()
			}
			m.logger.Error("subagent llm call failed",
				"iteration", iteration, "error", chatErr.Error())
			return "", fmt.Errorf("subagent llm call (iter %d): %w", iteration, chatErr)
		}
		if ctx.Err() != nil {
			// 流正常返回但 ctx 已取消（如 provider 在取消后关闭流）：
			// 不得把取消当成功继续循环，交由执行域安装方映射文案
			return "", ctx.Err()
		}

		// 收集本轮 LLM 输出
		var content string
		var toolCalls []*proto.ToolCall
		for _, f := range col.frames {
			content += f.Content
			if len(f.ToolCalls) > 0 {
				toolCalls = f.ToolCalls
			}
		}

		// 进度日志（不直接进 TUI 流以避免污染主 agent 会话日志）：
		// 用户可在 /plugins/logs SSE 或日志文件中实时看到子代理活动
		toolSummary := ""
		if len(toolCalls) > 0 {
			names := make([]string, 0, len(toolCalls))
			for _, tc := range toolCalls {
				names = append(names, tc.Name)
			}
			toolSummary = strings.Join(names, ",")
		}
		m.logger.Info("subagent progress",
			"iteration", iteration,
			"content_len", len(content),
			"tool_calls", toolSummary,
			"finish_reason", finishReasonOf(col.frames))

		assistantMsg := &proto.Message{Role: "assistant", Content: content}
		if len(toolCalls) > 0 {
			assistantMsg.ToolCalls = toolCalls
		}
		msgs = append(msgs, assistantMsg)

		// 无工具调用：模型自然收尾
		if len(toolCalls) == 0 {
			m.logger.Info("subagent completed",
				"iterations", iteration,
				"final_content_len", len(content))
			finalResult = content
			return finalResult, nil
		}

		// 执行每个工具调用：经宿主工具流水线（含沙箱/超时）。
		// toolCtx 注入 ApprovalPolicy=never，子代理不经审批人。
		for idx, tc := range toolCalls {
			callID := tc.Id
			if callID == "" {
				callID = fmt.Sprintf("subagent_%d", idx) // 部分 provider 不返回 id，补齐以关联结果
			}
			tc.Id = callID
			m.logger.Info("subagent executing tool",
				"iteration", iteration, "tool", tc.Name, "call_id", callID)
			result, err := m.ExecuteTool(toolCtx, tc.Name, json.RawMessage(tc.ArgumentsJson))
			TouchActivity(ctx) // 工具执行完成也算活动
			resultStr := result
			if err != nil {
				resultStr = "Error executing tool: " + err.Error()
				m.logger.Warn("subagent tool error",
					"iteration", iteration, "tool", tc.Name, "error", err.Error())
			}
			msgs = append(msgs, &proto.Message{Role: "tool", Content: resultStr, ToolCallId: callID})
		}
		iteration++
	}
}

// subagentSystemPrompt 构造子代理的 system 提示词。
// 子代理的权限范围与主 agent 一致（沿用当前会话沙箱策略），但审批一律 never：
// 任何被沙箱 fail-closed 拒绝的写操作都会直接返回错误，子代理不应重试，应把限制
// 写进最终结果交回主 agent 处理。
func subagentSystemPrompt(maxIter int) string {
	iterClause := "There is no iteration limit on you; decide when the task is done."
	if maxIter > 0 {
		iterClause = fmt.Sprintf("You have at most %d model-tool rounds. "+
			"If you cannot finish within that budget, return the best partial result with a brief note on what remains. "+
			"Prefer to converge: each tool call should make concrete progress, not repeat earlier steps.", maxIter)
	}
	return "You are a delegated subagent executing a task for a parent agent. " +
		"Your permission scope is the session's file/sandbox policy and cannot be widened from inside this task; " +
		"an operation denied by that policy is rejected automatically (approval is set to 'never' for subagent calls). " +
		"If an operation is denied, do not retry it — " +
		"state the limitation in your final result so the delegating agent can handle it. " +
		"Complete the task using the available tools if needed, then return only the final result, concise. " +
		"You decide when the task is done; there is no iteration limit on you by default. " +
		iterClause
}

// finishReasonOf 从收集到的帧中提取最后的 finish_reason（若有）。
func finishReasonOf(frames []*proto.ChatStreamResponse) string {
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i] != nil && frames[i].FinishReason != "" {
			return frames[i].FinishReason
		}
	}
	return ""
}

// frameCollector 内部收集流式帧，供子代理循环以流式路径调用聚合 LLM 服务。
// ctx 透传调用方（主 agent）的上下文：聚合 LLM 服务的 ChatStream 用
// stream.Context() 作为 provider 请求 ctx，若不透传则取消失效——主 agent 被
// 中断后子代理的 LLM 请求仍会挂起等待流，表现为「卡死/像超时无响应」。
//
// 每收到一帧就 TouchActivity(ctx)——向活跃续命执行域上报活动
// （持续生成时不会误杀）；ctx 未携带执行域时为 no-op。
type frameCollector struct {
	ctx    context.Context
	frames []*proto.ChatStreamResponse
}

func (c *frameCollector) Send(r *proto.ChatStreamResponse) error {
	c.frames = append(c.frames, r)
	TouchActivity(c.ctx) // 每帧到达都续命
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
		"so it suits long-running work; set max_iterations to impose an explicit cap if needed. " +
		"Subagent tool calls use approval=never (no interactive prompts); " +
		"an idle timeout (activity keeps it alive; default budget 10m, configurable via " +
		"DSC_SUBAGENT_IDLE_TIMEOUT in the timeout policy) protects against hung LLM providers " +
		"while allowing slow local models to keep running."
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
