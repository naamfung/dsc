package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"dsc/plugin"
	openai "github.com/sashabaranov/go-openai"
	gp "github.com/hashicorp/go-plugin"
)

type OpenAIProvider struct {
	client *openai.Client
	model  string
}

// toolCallDeltaAccumulator 用於累積工具調用的增量信息
type toolCallDeltaAccumulator struct {
	ID           string
	Name         string
	ArgumentsStr string
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []plugin.Message, tools []plugin.Tool) (*plugin.ChatResponse, error) {
	// 转换消息格式
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		msg := openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
		// assistant 消息需回带工具调用（OpenAI 格式要求 tool_calls 与后续 tool 结果匹配）
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]openai.ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				msg.ToolCalls[j] = openai.ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				}
			}
		}
		openaiMessages[i] = msg
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
				ID:        tc.ID,
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

// ChatStream 實現 LLMProvider.ChatStream 接口
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []plugin.Message, tools []plugin.Tool) (<-chan *plugin.ChatStreamResponse, error) {
	// 轉換消息格式
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		msg := openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
		// assistant 消息需回带工具调用
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]openai.ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				msg.ToolCalls[j] = openai.ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: string(argsJSON),
					},
				}
			}
		}
		openaiMessages[i] = msg
	}

	// 轉換工具格式
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
		Stream:   true,
	}

	stream, err := p.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan *plugin.ChatStreamResponse)
	go func() {
		defer close(ch)

		var textAccumulator strings.Builder
		var toolCallAccums map[int]*toolCallDeltaAccumulator

		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- &plugin.ChatStreamResponse{Error: err.Error()}
				return
			}

			if len(resp.Choices) == 0 {
				continue
			}

			choice := resp.Choices[0]
			delta := choice.Delta

			// 處理文本增量
			if delta.Content != "" {
				textAccumulator.WriteString(delta.Content)
				ch <- &plugin.ChatStreamResponse{
					Content: delta.Content,
				}
			}

			// 處理工具調用增量
			if len(delta.ToolCalls) > 0 {
				if toolCallAccums == nil {
					toolCallAccums = make(map[int]*toolCallDeltaAccumulator)
				}
				for _, tc := range delta.ToolCalls {
					idx := 0
					if tc.Index != nil {
						idx = *tc.Index
					}
					if toolCallAccums[idx] == nil {
						toolCallAccums[idx] = &toolCallDeltaAccumulator{
							ID: tc.ID,
						}
					}
					acc := toolCallAccums[idx]
					if tc.Function.Name != "" {
						acc.Name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						acc.ArgumentsStr += tc.Function.Arguments
					}
				}
			}

			// 處理完成原因
			if choice.FinishReason != "" && choice.FinishReason != "null" {
				// 轉換工具調用為 plugin.ToolCall
				var toolCalls []plugin.ToolCall
				for _, acc := range toolCallAccums {
					var args map[string]interface{}
					json.Unmarshal([]byte(acc.ArgumentsStr), &args)
					toolCalls = append(toolCalls, plugin.ToolCall{
						ID:        acc.ID,
						Name:      acc.Name,
						Arguments: args,
					})
				}

				finishReason := string(choice.FinishReason)
				if finishReason == "null" {
					finishReason = "stop"
				}

				ch <- &plugin.ChatStreamResponse{
					Content:      "",
					FinishReason: finishReason,
					ToolCalls:    toolCalls,
				}
				return
			}
		}
	}()

	return ch, nil
}

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		// 對於 llama.cpp server，API key 通常是可選的或接受任意值
		apiKey = "sk-laamaafung-not-used"
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "deepseek-v4-flash"
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
