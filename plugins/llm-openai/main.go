package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sort"
	"strings"

	"dsc-sdk"
	"dsc/core"
	openai "github.com/sashabaranov/go-openai"
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

// usageFromOpenAI 將 OpenAI usage 轉換為 core.Usage（nil 返回 nil）。
// cached_tokens（DeepSeek/llama.cpp 的 prompt_tokens_details.cached_tokens）映射为
// CacheReadInputTokens；命中率计算需要 miss 侧，按 REX 语义以
// CacheCreationInputTokens = prompt - cached 近似（无缓存报告时保持 0）。
func usageFromOpenAI(u *openai.Usage) *core.Usage {
	if u == nil {
		return nil
	}
	var cacheRead int32
	if u.PromptTokensDetails != nil {
		cacheRead = int32(u.PromptTokensDetails.CachedTokens)
	}
	cacheMiss := int32(u.PromptTokens) - cacheRead
	if cacheMiss < 0 {
		cacheMiss = 0
	}
	return &core.Usage{
		PromptTokens:             int32(u.PromptTokens),
		CompletionTokens:         int32(u.CompletionTokens),
		TotalTokens:              int32(u.TotalTokens),
		CacheReadInputTokens:     cacheRead,
		CacheCreationInputTokens: cacheMiss,
	}
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []core.Message, tools []core.Tool, maxTokens int) (*core.ChatResponse, error) {
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
	if maxTokens > 0 {
		req.MaxTokens = maxTokens
	}

	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return &core.ChatResponse{Content: "", FinishReason: "stop"}, nil
	}

	choice := resp.Choices[0]
	result := &core.ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: string(choice.FinishReason),
	}

	// 处理工具调用
	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]core.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			var args map[string]interface{}
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
			result.ToolCalls[i] = core.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			}
		}
	}

	return result, nil
}

func (p *OpenAIProvider) Name(ctx context.Context) string       { return "openai" }
func (p *OpenAIProvider) Version(ctx context.Context) string    { return "1.0.0" }
func (p *OpenAIProvider) HealthCheck(ctx context.Context) error { return nil }

// ChatStream 實現 LLMProvider.ChatStream 接口
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan *core.ChatStreamResponse, error) {
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
		// 请求流式 usage：服务端（含 llama.cpp）会在最后一个分片返回整轮 token 统计
		StreamOptions: &openai.StreamOptions{IncludeUsage: true},
	}

	stream, err := p.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan *core.ChatStreamResponse)
	go func() {
		defer close(ch)

		var textAccumulator strings.Builder
		var toolCallAccums map[int]*toolCallDeltaAccumulator
		streamFinished := false

		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- &core.ChatStreamResponse{Error: err.Error()}
				return
			}

			// 處理 usage 數據（llama.cpp 等會在流式響應中攜帶 usage）
			if resp.Usage != nil {
				ch <- &core.ChatStreamResponse{
					Usage: usageFromOpenAI(resp.Usage),
				}
			}

			if len(resp.Choices) == 0 {
				// llama.cpp 在正文結束後會追加一個空 choices、僅含 usage 的結尾分片；
				// 該分片送達即代表流已結束，處理完 usage 即可退出。
				if streamFinished {
					return
				}
				continue
			}

			choice := resp.Choices[0]
			delta := choice.Delta

			// 處理文本增量
			if delta.Content != "" {
				textAccumulator.WriteString(delta.Content)
				ch <- &core.ChatStreamResponse{
					Content: delta.Content,
				}
			}

			// 處理思考過程增量（DeepSeek reasoning_content 等）
			if delta.ReasoningContent != "" {
				ch <- &core.ChatStreamResponse{
					Reasoning: delta.ReasoningContent,
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
					// 部分服务端（如 llama.cpp）在后续分片才携带 ID，迟到时补上，
					// 避免工具调用 ID 为空导致 agent 无法关联 tool result。
					if acc.ID == "" && tc.ID != "" {
						acc.ID = tc.ID
					}
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
				if streamFinished {
					continue
				}
				streamFinished = true

				// 轉換工具調用為 core.ToolCall：按 index 排序，保证多工具调用顺序稳定
				// （map 遍历顺序随机，直接遍历会让同一流在不同运行下顺序漂移）。
				idxList := make([]int, 0, len(toolCallAccums))
				for idx := range toolCallAccums {
					idxList = append(idxList, idx)
				}
				sort.Ints(idxList)
				var toolCalls []core.ToolCall
				for _, idx := range idxList {
					acc := toolCallAccums[idx]
					var args map[string]interface{}
					json.Unmarshal([]byte(acc.ArgumentsStr), &args)
					toolCalls = append(toolCalls, core.ToolCall{
						ID:        acc.ID,
						Name:      acc.Name,
						Arguments: args,
					})
				}

				finishReason := string(choice.FinishReason)

				// 把 usage 一并转发（部分服務會在 finish 分片攜帶 usage）；
				// 若該分片同時含正文/工具調用，則轉發完後仍繼續讀流，
				// 由後續的空 choices 結尾分片（或 io.EOF）收尾退出。
				ch <- &core.ChatStreamResponse{
					Content:      "",
					FinishReason: finishReason,
					ToolCalls:    toolCalls,
					Usage:        usageFromOpenAI(resp.Usage),
				}
				// 不在這裡返回：llama.cpp 等會緊接著發送僅含 usage 的空 choices 結尾分片，
				// 稍後由上方空 choices 分支收尾退出。
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

	// 以公共 SDK（dsc-sdk）声明式启动：SDK 复用宿主 core.LLMGRPCPlugin
	// 自动提供 LLMService + 元数据（重写自旧的 plugin.Serve 样板）。
	sdk := dsc.New(dsc.Config{Name: "openai", Version: "1.0.0", Type: dsc.TypeLLM})
	sdk.LLM(provider)
	sdk.Serve()
}
