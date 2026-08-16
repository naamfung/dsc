package main

import (
	"context"
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
	// Anthropic 插件尚未支持工具調用
	if len(tools) > 0 {
		return nil, fmt.Errorf("Anthropic plugin does not support tool calls yet")
	}

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
		// 注意：Anthropic 不支持 tool 消息角色，需要特殊處理（將 tool 響應作為 user 或 assistant 部分）
		default:
			// 默認為 user
			userMessages = append(userMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		}
	}

	params := anthropic.MessageNewParams{
		MaxTokens: 1024,
		Model:     anthropic.Model(p.model),
		Messages:  userMessages,
	}
	if systemMsg != "" {
		params.System = []anthropic.TextBlockParam{
			{
				Text: systemMsg,
				Type: "text",
			},
		}
	}

	// 如果提供了工具，可轉換為 Anthropic 的 tool 參數（此處省略，保持與現有功能一致）
	// 如果需要，可參考 SDK 文檔添加 Tools 字段

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}

	var content string
	for _, block := range resp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return &plugin.ChatResponse{
		Content:      content,
		FinishReason: string(resp.StopReason),
		ToolCalls:    nil, // 暫不處理工具調用
	}, nil
}

func (p *AnthropicProvider) Name(ctx context.Context) string { return "anthropic" }
func (p *AnthropicProvider) Version(ctx context.Context) string { return "1.0.0" }
func (p *AnthropicProvider) HealthCheck(ctx context.Context) error { return nil }

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		// 對於 llama.cpp server，API key 通常是可選的或接受任意值
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