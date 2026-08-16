package main

import (
	"context"
	"encoding/json"
	"os"

	"dsc/plugin"
	openai "github.com/sashabaranov/go-openai"
	gp "github.com/hashicorp/go-plugin"
)

type OpenAIProvider struct {
	client *openai.Client
	model  string
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []plugin.Message, tools []plugin.Tool) (*plugin.ChatResponse, error) {
	// 转换消息格式
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	// 转换工具格式
	openaiTools := make([]openai.Tool, len(tools))
	for i, t := range tools {
		openaiTools[i] = openai.Tool{
			Type: "function",
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  json.RawMessage(t.ParametersJSON),
			},
		}
	}

	req := openai.ChatCompletionRequest{
		Model:    p.model,
		Messages: openaiMessages,
		Tools:    openaiTools,
	}

	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return &plugin.ChatResponse{Content: "", FinishReason: "stop"}, nil
	}

	choice := resp.Choices[0]
	result := &plugin.ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: string(choice.FinishReason),
	}

	// 处理工具调用
	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]plugin.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			var args map[string]interface{}
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
			result.ToolCalls[i] = plugin.ToolCall{
				Name:      tc.Function.Name,
				Arguments: args,
			}
		}
	}

	return result, nil
}

func (p *OpenAIProvider) Name(ctx context.Context) string { return "openai" }
func (p *OpenAIProvider) Version(ctx context.Context) string { return "1.0.0" }
func (p *OpenAIProvider) HealthCheck(ctx context.Context) error { return nil }

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		// 對於 llama.cpp server，API key 通常是可選的或接受任意值
		apiKey = "sk-llama-cpp-not-used"
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "Agentic-Turbo-Coder" // 根據 llama.cpp 返回的模型名稱
	}

	provider := &OpenAIProvider{
		client: openai.NewClientWithConfig(config),
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
