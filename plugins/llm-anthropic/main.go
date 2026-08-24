package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"dsc-sdk"
	"dsc/plugin"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type AnthropicProvider struct {
	client anthropic.Client
	model  string
	// thinking 是否启用扩展思考（extended thinking）。默认开启（DeepSeek anthropic 接口
	// 支持并返回 thinking 块）；ANTHROPIC_THINKING=0 可关闭。
	thinking       bool
	thinkingBudget int64
	// maxTokens 单轮输出上限。为「不应人为限制」起见默认取较大值，仅当显式配置时才收紧；
	// 流式路径（TUI / -input）以该值为准，非流式 Chat 则在其 >0 时覆盖。
	maxTokens int64
}

// buildMessageParams 构建 Anthropic 请求参数（消息、system、工具定义）。
// maxTokens <= 0 表示使用服务端默认（不再人为限制）；>0 时才显式携带。
func (p *AnthropicProvider) buildMessageParams(messages []plugin.Message, tools []plugin.Tool, maxTokens int64) anthropic.MessageNewParams {
	// 1. 构建 Anthropic 消息
	var systemMsg string
	userMessages := make([]anthropic.MessageParam, 0, len(messages))

	for _, m := range messages {
		switch m.Role {
		case "system":
			systemMsg = m.Content
		case "user":
			userMessages = append(userMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case "assistant":
			// Anthropic 要求：若后续跟随 tool_result（tool 消息），本 assistant 消息必须
			// 回带对应的 tool_use 块，否则 API 无法把工具结果关联到之前的调用
			//（REX 同样回显 tool_use）。无文本且无工具调用时跳过，避免空内容消息。
			var blocks []anthropic.ContentBlockParamUnion
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			for _, tc := range m.ToolCalls {
				input, err := json.Marshal(tc.Arguments)
				if err != nil || len(input) == 0 {
					input = []byte("{}")
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, json.RawMessage(input), tc.Name))
			}
			if len(blocks) > 0 {
				userMessages = append(userMessages, anthropic.NewAssistantMessage(blocks...))
			}
		case "tool":
			// Anthropic 要求 tool 结果以 user 消息发送，并附上对应的 tool_use_id
			// 如果 m.ToolCallID 为空，降级为纯文本（兼容旧逻辑）
			if m.ToolCallID != "" {
				userMessages = append(userMessages, anthropic.NewUserMessage(
					anthropic.NewToolResultBlock(m.ToolCallID, m.Content, false),
				))
			} else {
				userMessages = append(userMessages, anthropic.NewUserMessage(
					anthropic.NewTextBlock(fmt.Sprintf("Tool result: %s", m.Content)),
				))
			}
		default:
			userMessages = append(userMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		}
	}

	// 2. 转换工具定义
	var toolParams []anthropic.ToolUnionParam
	if len(tools) > 0 {
		for _, t := range tools {
			// 直接反序列化 JSON Schema 為 ToolInputSchemaParam，
			// 避免把完整 schema 錯誤地包進 properties 導致服務端解析失敗
			var inputSchema anthropic.ToolInputSchemaParam
			if err := json.Unmarshal([]byte(t.ParametersJSON), &inputSchema); err != nil {
				// 若 schema 解析失败，使用空对象（type 默認為 object）
				inputSchema = anthropic.ToolInputSchemaParam{}
			}
			toolParams = append(toolParams, anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					Name:        t.Name,
					InputSchema: inputSchema,
					Description: anthropic.String(t.Description),
				},
			})
		}
	}

	// 3. 构建请求参数
	msgParams := anthropic.MessageNewParams{
		Model:    anthropic.Model(p.model),
		Messages: userMessages,
	}
	// max_tokens：仅当显式给定 >0 时设置；否则交给服务端默认（不再有写死 1024 的人为截断）
	if maxTokens > 0 {
		msgParams.MaxTokens = maxTokens
	}
	if systemMsg != "" {
		msgParams.System = []anthropic.TextBlockParam{
			{Text: systemMsg, Type: "text"},
		}
	}
	if len(toolParams) > 0 {
		msgParams.Tools = toolParams
	}
	// 4. 扩展思考（extended thinking）：启用后模型会返回 thinking 块，
	//    其内容经 chat 流式增量转发为 reasoning 帧最终渲染到 TUI。
	if p.thinking {
		msgParams.Thinking = anthropic.ThinkingConfigParamOfEnabled(p.thinkingBudget)
	}
	return msgParams
}

// concatText 拼接消息中所有 text 块的内容
func concatText(blocks []anthropic.ContentBlockUnion) string {
	var b strings.Builder
	for _, block := range blocks {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

// concatReasoning 拼接消息中所有 thinking 块的内容（思考过程）
func concatReasoning(blocks []anthropic.ContentBlockUnion) string {
	var b strings.Builder
	for _, block := range blocks {
		// 嘗試作為 thinking 塊訪問
		if thinkingBlock := block.AsThinking(); thinkingBlock.Thinking != "" {
			b.WriteString(thinkingBlock.Thinking)
		}
	}
	return b.String()
}

// extractToolCalls 从消息内容块中提取工具调用
func extractToolCalls(blocks []anthropic.ContentBlockUnion) []plugin.ToolCall {
	var toolCalls []plugin.ToolCall
	for _, block := range blocks {
		if block.Type != "tool_use" {
			continue
		}
		toolUse := block.AsToolUse()
		argsMap := make(map[string]any)
		if err := json.Unmarshal(toolUse.Input, &argsMap); err != nil {
			var rawArgs map[string]any
			if err2 := json.Unmarshal(toolUse.Input, &rawArgs); err2 == nil {
				argsMap = rawArgs
			}
		}
		toolCalls = append(toolCalls, plugin.ToolCall{
			ID:        toolUse.ID,
			Name:      toolUse.Name,
			Arguments: argsMap,
		})
	}
	return toolCalls
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []plugin.Message, tools []plugin.Tool, maxTokens int) (*plugin.ChatResponse, error) {
	params := p.buildMessageParams(messages, tools, int64(maxTokens))
	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}

	return &plugin.ChatResponse{
		Content:      concatText(resp.Content),
		FinishReason: string(resp.StopReason),
		ToolCalls:    extractToolCalls(resp.Content),
	}, nil
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, messages []plugin.Message, tools []plugin.Tool) (<-chan *plugin.ChatStreamResponse, error) {
	stream := p.client.Messages.NewStreaming(ctx, p.buildMessageParams(messages, tools, p.maxTokens))

	ch := make(chan *plugin.ChatStreamResponse)
	go func() {
		defer close(ch)
		acc := newStreamAccumulator()
		prevLen := 0
		prevReasonLen := 0
		for stream.Next() {
			event := stream.Current()
			if err := acc.accumulate(event); err != nil {
				ch <- &plugin.ChatStreamResponse{Error: fmt.Sprintf("stream accumulate error: %v", err)}
				return
			}
			// 仅当文本有新增时才发送增量帧
			text := concatText(acc.msg.Content)
			if len(text) > prevLen {
				ch <- &plugin.ChatStreamResponse{Content: text[prevLen:]}
				prevLen = len(text)
			}
			// 思考过程增量（thinking 块）
			reason := concatReasoning(acc.msg.Content)
			if len(reason) > prevReasonLen {
				// [DEBUG] 打印 reasoning 帧
				fmt.Fprintf(os.Stderr, "[LLM-ANTHROPIC-REASONING] %s\n", reason[prevReasonLen:])
				ch <- &plugin.ChatStreamResponse{Reasoning: reason[prevReasonLen:]}
				prevReasonLen = len(reason)
			}
		}
		if err := stream.Err(); err != nil {
			ch <- &plugin.ChatStreamResponse{Error: err.Error()}
			return
		}
		// 流结束：发送最终帧（工具调用与结束原因；文本已增量发送，不再重复）
		resp := &plugin.ChatStreamResponse{
			FinishReason: string(acc.msg.StopReason),
			ToolCalls:    extractToolCalls(acc.msg.Content),
		}
		// 處理 usage 信息（Anthropic 格式：input_tokens -> prompt_tokens, output_tokens -> completion_tokens；
		// cache_read/cache_creation 直接对应）
		if acc.msg.Usage.InputTokens > 0 || acc.msg.Usage.OutputTokens > 0 {
			resp.Usage = &plugin.Usage{
				PromptTokens:             int32(acc.msg.Usage.InputTokens),
				CompletionTokens:         int32(acc.msg.Usage.OutputTokens),
				TotalTokens:              int32(acc.msg.Usage.InputTokens + acc.msg.Usage.OutputTokens),
				CacheReadInputTokens:     int32(acc.msg.Usage.CacheReadInputTokens),
				CacheCreationInputTokens: int32(acc.msg.Usage.CacheCreationInputTokens),
			}
		}
		ch <- resp
	}()
	return ch, nil
}

func (p *AnthropicProvider) Name(ctx context.Context) string {
	return "anthropic"
}

func (p *AnthropicProvider) Version(ctx context.Context) string {
	return "1.1.0" // 版本升级，支持工具调用
}

func (p *AnthropicProvider) HealthCheck(ctx context.Context) error {
	return nil
}

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		// 对于 llama.cpp server 或测试，可忽略
		apiKey = "sk-laamaafung-not-used"
	}
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com/anthropic"
	}
	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = "deepseek-v4-flash"
	}

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	// 扩展思考默认开启：DeepSeek 的 anthropic 接口支持 thinking 参数并返回 thinking 块，
	// 其内容经 chat 流式增量转发为 reasoning 帧渲染到 TUI。
	// ANTHROPIC_THINKING=0 可显式关闭（用于不返回 thinking 的普通模型）；
	// ANTHROPIC_THINKING_BUDGET 设定思考 token 预算（默认 4096；DeepSeek 忽略 budget 值）。
	thinking := os.Getenv("ANTHROPIC_THINKING") != "0"
	thinkingBudget := int64(4096)
	if v := os.Getenv("ANTHROPIC_THINKING_BUDGET"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			thinkingBudget = n
		}
	}
	// 单轮输出上限：默认取较大值（32768）视为「不人为限制」；可用 ANTHROPIC_MAX_OUTPUT_TOKENS 收紧。
	// 且不低于 thinking budget，避免扩展思考下 max_tokens 被预算吞掉导致生成过早停止。
	maxTokens := int64(32768)
	if v := os.Getenv("ANTHROPIC_MAX_OUTPUT_TOKENS"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > thinkingBudget {
			maxTokens = n
		}
	}

	provider := &AnthropicProvider{
		client:         anthropic.NewClient(opts...),
		model:          model,
		thinking:       thinking,
		thinkingBudget: thinkingBudget,
		maxTokens:      maxTokens,
	}

	// 以公共 SDK（dsc-sdk）声明式启动：SDK 复用宿主 plugin.LLMGRPCPlugin
	// 自动提供 LLMService + 元数据（重写自旧的 goplugin.Serve 样板）。
	sdk := dsc.New(dsc.Config{Name: "anthropic", Version: "1.1.0", Type: dsc.TypeLLM})
	sdk.LLM(provider)
	sdk.Serve()
}
