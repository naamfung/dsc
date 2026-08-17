package main

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"

	"dsc/plugin"
	"github.com/ollama/ollama/api"
	gp "github.com/hashicorp/go-plugin"
)

func boolPtr(b bool) *bool {
	return &b
}

type OllamaProvider struct {
	client *api.Client
	model  string
}

func (p *OllamaProvider) Chat(ctx context.Context, messages []plugin.Message, tools []plugin.Tool) (*plugin.ChatResponse, error) {
	// 转换消息
	ollamaMessages := make([]api.Message, len(messages))
	for i, m := range messages {
		msg := api.Message{Role: m.Role, Content: m.Content}
		// assistant 消息需回带工具调用，否则 ollama 无法关联后续 tool 结果
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]api.ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				var ollamaArgs api.ToolCallFunctionArguments
				json.Unmarshal(argsJSON, &ollamaArgs)
				msg.ToolCalls[j] = api.ToolCall{
					ID: tc.ID,
					Function: api.ToolCallFunction{
						Name:      tc.Name,
						Arguments: ollamaArgs,
					},
				}
			}
		}
		ollamaMessages[i] = msg
	}

	// 转换工具（如果提供）
	ollamaTools := make([]api.Tool, len(tools))
	for i, t := range tools {
		// 解析 JSON Schema
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(t.ParametersJSON), &params); err != nil {
			// 若解析失敗，使用空對象
			params = map[string]interface{}{
				"type": "object",
			}
		}

		// 轉換 properties 為 *api.ToolPropertiesMap
		var props *api.ToolPropertiesMap
		if propsMap, ok := params["properties"].(map[string]interface{}); ok && propsMap != nil {
			propsBytes, err := json.Marshal(propsMap)
			if err == nil {
				props = new(api.ToolPropertiesMap)
				json.Unmarshal(propsBytes, props)
			}
		}

		// 轉換 required 為 []string
		var required []string
		if reqList, ok := params["required"].([]interface{}); ok {
			for _, r := range reqList {
				if rStr, ok := r.(string); ok {
					required = append(required, rStr)
				}
			}
		} else if reqList, ok := params["required"].([]string); ok {
			required = reqList
		}

		ollamaTools[i] = api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: props,
					Required:   required,
				},
			},
		}
	}

	req := api.ChatRequest{
		Model:    p.model,
		Messages: ollamaMessages,
		Tools:    ollamaTools,
	}

	var resp api.ChatResponse
	err := p.client.Chat(ctx, &req, func(c api.ChatResponse) error {
		resp = c
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 處理工具調用
	var toolCalls []plugin.ToolCall
	if len(resp.Message.ToolCalls) > 0 {
		for _, tc := range resp.Message.ToolCalls {
			var args map[string]interface{}
			if argsJSON, err := json.Marshal(tc.Function.Arguments); err == nil {
				json.Unmarshal(argsJSON, &args)
			}
			toolCalls = append(toolCalls, plugin.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}
	}

	return &plugin.ChatResponse{
		Content:      resp.Message.Content,
		FinishReason: "stop",
		ToolCalls:    toolCalls,
	}, nil
}

func (p *OllamaProvider) Name(ctx context.Context) string { return "ollama" }
func (p *OllamaProvider) Version(ctx context.Context) string { return "1.0.0" }
func (p *OllamaProvider) HealthCheck(ctx context.Context) error { return nil }

// ChatStream 實現 LLMProvider.ChatStream 接口
func (p *OllamaProvider) ChatStream(ctx context.Context, messages []plugin.Message, tools []plugin.Tool) (<-chan *plugin.ChatStreamResponse, error) {
	// 轉換消息
	ollamaMessages := make([]api.Message, len(messages))
	for i, m := range messages {
		msg := api.Message{Role: m.Role, Content: m.Content}
		// assistant 消息需回带工具调用
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]api.ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				var ollamaArgs api.ToolCallFunctionArguments
				json.Unmarshal(argsJSON, &ollamaArgs)
				msg.ToolCalls[j] = api.ToolCall{
					ID: tc.ID,
					Function: api.ToolCallFunction{
						Name:      tc.Name,
						Arguments: ollamaArgs,
					},
				}
			}
		}
		ollamaMessages[i] = msg
	}

	// 轉換工具（如果提供）
	ollamaTools := make([]api.Tool, len(tools))
	for i, t := range tools {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(t.ParametersJSON), &params); err != nil {
			params = map[string]interface{}{
				"type": "object",
			}
		}

		var props *api.ToolPropertiesMap
		if propsMap, ok := params["properties"].(map[string]interface{}); ok && propsMap != nil {
			propsBytes, err := json.Marshal(propsMap)
			if err == nil {
				props = new(api.ToolPropertiesMap)
				json.Unmarshal(propsBytes, props)
			}
		}

		var required []string
		if reqList, ok := params["required"].([]interface{}); ok {
			for _, r := range reqList {
				if rStr, ok := r.(string); ok {
					required = append(required, rStr)
				}
			}
		} else if reqList, ok := params["required"].([]string); ok {
			required = reqList
		}

		ollamaTools[i] = api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: props,
					Required:   required,
				},
			},
		}
	}

	req := api.ChatRequest{
		Model:    p.model,
		Messages: ollamaMessages,
		Tools:    ollamaTools,
		Stream:   boolPtr(true),
	}

	ch := make(chan *plugin.ChatStreamResponse)
	go func() {
		defer close(ch)

		var textAccumulator strings.Builder
		var toolCallAccums map[int]*toolCallDeltaAccumulatorOllama

		streamErr := p.client.Chat(ctx, &req, func(resp api.ChatResponse) error {
			// 處理文本增量
			if resp.Message.Content != "" {
				textAccumulator.WriteString(resp.Message.Content)
				ch <- &plugin.ChatStreamResponse{
					Content: resp.Message.Content,
				}
			}

			// 處理工具調用增量
			if len(resp.Message.ToolCalls) > 0 {
				if toolCallAccums == nil {
					toolCallAccums = make(map[int]*toolCallDeltaAccumulatorOllama)
				}
				for _, tc := range resp.Message.ToolCalls {
					idx := 0
					// Ollama 的 ToolCall 沒有 Index 字段，使用固定索引或根據 ID 區分
					if tc.ID != "" {
						// 簡化處理：將所有 tool call 累積到同一個索引
						idx = 0
					}
					if toolCallAccums[idx] == nil {
						toolCallAccums[idx] = &toolCallDeltaAccumulatorOllama{
							ID: tc.ID,
						}
					}
					acc := toolCallAccums[idx]
					if tc.Function.Name != "" {
						acc.Name = tc.Function.Name
					}
					if tc.Function.Arguments != (api.ToolCallFunctionArguments{}) {
						argsJSON, _ := json.Marshal(tc.Function.Arguments)
						acc.ArgumentsStr += string(argsJSON)
					}
				}
			}

			// 處理完成原因
			if resp.Done {
				var toolCalls []plugin.ToolCall
				for _, acc := range toolCallAccums {
					var args map[string]interface{}
					if acc.ArgumentsStr != "" {
						json.Unmarshal([]byte(acc.ArgumentsStr), &args)
					}
					toolCalls = append(toolCalls, plugin.ToolCall{
						ID:        acc.ID,
						Name:      acc.Name,
						Arguments: args,
					})
				}

				ch <- &plugin.ChatStreamResponse{
					Content:      "",
					FinishReason: "stop",
					ToolCalls:    toolCalls,
				}
				return nil
			}

			return nil
		})

		if streamErr != nil {
			ch <- &plugin.ChatStreamResponse{Error: streamErr.Error()}
		}
	}()

	return ch, nil
}

type toolCallDeltaAccumulatorOllama struct {
	ID           string
	Name         string
	ArgumentsStr string
}

func main() {
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "llama3.1:latest"
	}

	hostStr := os.Getenv("OLLAMA_HOST")
	if hostStr == "" {
		hostStr = "http://127.0.0.1:11434"
	}
	hostURL, _ := url.Parse(hostStr)

	provider := &OllamaProvider{
		client: api.NewClient(hostURL, nil),
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