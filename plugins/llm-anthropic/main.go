package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"dsc/plugin"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	gp "github.com/hashicorp/go-plugin"
)

type AnthropicProvider struct {
	client anthropic.Client
	model  string
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []plugin.Message, tools []plugin.Tool) (*plugin.ChatResponse, error) {
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
			userMessages = append(userMessages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
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
			var inputSchema map[string]any
			if err := json.Unmarshal([]byte(t.ParametersJSON), &inputSchema); err != nil {
				// 若 schema 解析失败，使用空对象
				inputSchema = map[string]any{"type": "object"}
			}
			toolParams = append(toolParams, anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					Name: t.Name,
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: inputSchema,
					},
					Description: anthropic.String(t.Description),
				},
			})
		}
	}

	// 3. 构建请求参数
	msgParams := anthropic.MessageNewParams{
		MaxTokens: 1024,
		Model:     anthropic.Model(p.model),
		Messages:  userMessages,
	}
	if systemMsg != "" {
		msgParams.System = []anthropic.TextBlockParam{
			{Text: systemMsg, Type: "text"},
		}
	}
	if len(toolParams) > 0 {
		msgParams.Tools = toolParams
	}

	// 4. 调用 API
	resp, err := p.client.Messages.New(ctx, msgParams)
	if err != nil {
		return nil, err
	}

	// 5. 解析响应
	var content string
	var toolCalls []plugin.ToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			toolUse := block.AsToolUse()
			argsMap := make(map[string]any)
			if err := json.Unmarshal(toolUse.Input, &argsMap); err != nil {
				// 若 JSON 解析失敗，嘗試直接轉換
				var rawArgs map[string]any
				if err2 := json.Unmarshal(toolUse.Input, &rawArgs); err2 == nil {
					argsMap = rawArgs
				}
			}
			toolCalls = append(toolCalls, plugin.ToolCall{
				Name:      toolUse.Name,
				Arguments: argsMap,
			})
		}
	}

	return &plugin.ChatResponse{
		Content:      content,
		FinishReason: string(resp.StopReason),
		ToolCalls:    toolCalls,
	}, nil
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
		apiKey = "sk-llama-cpp-not-used"
	}
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = "claude-3-opus-20240229"
	}

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	provider := &AnthropicProvider{
		client: anthropic.NewClient(opts...),
		model:  model,
	}

	gp.Serve(&gp.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]gp.Plugin{
			"llm": &plugin.LLMGRPCPlugin{Impl: provider},
		},
		GRPCServer: gp.DefaultGRPCServer,
	})
}
