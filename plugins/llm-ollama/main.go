package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"dsc/plugin"
	gp "github.com/hashicorp/go-plugin"
	"github.com/ollama/ollama/api"
)

func boolPtr(b bool) *bool {
	return &b
}

type OllamaProvider struct {
	client *api.Client
	model  string
	// thinking 是否启用思考（thinking/reasoning 模型）。默认开启（与 llm-anthropic 一致）；
	// OLLAMA_THINKING=0 可关闭。开启后从响应 Message.Thinking 提取 reasoning 帧渲染到 TUI。
	thinking bool
}

func (p *OllamaProvider) Chat(ctx context.Context, messages []plugin.Message, tools []plugin.Tool, maxTokens int) (*plugin.ChatResponse, error) {
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
	if p.thinking {
		req.Think = &api.ThinkValue{Value: true}
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

func (p *OllamaProvider) Name(ctx context.Context) string       { return "ollama" }
func (p *OllamaProvider) Version(ctx context.Context) string    { return "1.0.0" }
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
	if p.thinking {
		req.Think = &api.ThinkValue{Value: true}
	}

	ch := make(chan *plugin.ChatStreamResponse)
	go func() {
		defer close(ch)

		var textAccumulator strings.Builder
		var toolCallAccums map[string]*toolCallDeltaAccumulatorOllama
		var toolCallOrder []string

		streamErr := p.client.Chat(ctx, &req, func(resp api.ChatResponse) error {
			// 處理思考過程增量（Message.Thinking；OLLAMA_THINKING=0 時不會出現，
			// 与 llm-anthropic/openai 一致：思考作为 reasoning 帧单独发送）
			if resp.Message.Thinking != "" {
				fmt.Fprintf(os.Stderr, "[LLM-OLLAMA-REASONING] %s\n", resp.Message.Thinking)
				ch <- &plugin.ChatStreamResponse{
					Reasoning: resp.Message.Thinking,
				}
			}

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
					toolCallAccums = make(map[string]*toolCallDeltaAccumulatorOllama)
				}
				for _, tc := range resp.Message.ToolCalls {
					// Ollama 的 ToolCallFunction 带 Index（服务端按工具解析序号分配，跨帧稳定）：
					// 以 Index 为主键分桶，同一调用跨帧去重；Index 与 ID 均缺失时按出现顺序
					// 分配键，避免多个工具调用合并到同一桶导致 Name 互相覆盖、Arguments
					// JSON 拼接后解析失败。
					key := fmt.Sprintf("tool_%d_%s", tc.Function.Index, tc.ID)
					if tc.Function.Index == 0 && tc.ID == "" {
						key = fmt.Sprintf("tool_anon_%d", len(toolCallAccums))
					}
					if _, ok := toolCallAccums[key]; !ok {
						toolCallOrder = append(toolCallOrder, key)
						toolCallAccums[key] = &toolCallDeltaAccumulatorOllama{
							ID: tc.ID,
						}
					}
					acc := toolCallAccums[key]
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
				for _, key := range toolCallOrder {
					acc := toolCallAccums[key]
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

	// 思考默认开启（与 llm-anthropic 的 ANTHROPIC_THINKING 一致）；OLLAMA_THINKING=0 关闭
	thinking := os.Getenv("OLLAMA_THINKING") != "0"

	provider := &OllamaProvider{
		client:   api.NewClient(hostURL, nil),
		model:    model,
		thinking: thinking,
	}

	gp.Serve(&gp.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]gp.Plugin{
			"llm": &plugin.LLMGRPCPlugin{Impl: provider},
		},
		GRPCServer: gp.DefaultGRPCServer,
	})
}
