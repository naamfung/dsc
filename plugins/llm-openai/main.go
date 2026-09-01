package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"sync"

	"dsc-sdk"
	"dsc/core"
	openai "github.com/sashabaranov/go-openai"
)

type OpenAIProvider struct {
	client *openai.Client
	model  string
	// vision 是否启用图像输入：默认开启（对齐 DSH），DSC_NO_VISION=1 可关闭。
	vision bool
	// filesAPI 是否可把超大图自动上传 DeepSeek Files API（base URL 为 deepseek.com 时启用）。
	filesAPI bool
	// fileCache 已上传图片的 data URL → file_id 缓存（同一图多轮复用，避免重复上传）。
	fileMu    sync.Mutex
	fileCache map[string]string
}

// maxInlineImageBytes 内联 base64 单图大小上限（对齐 DeepSeek 32 MiB 内联限制，
// 预留余量避免请求体逼近 48 MiB 上限；超出则走 Files API 上传）。
const maxInlineImageBytes = 20 << 20 // 20 MiB

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

// visionEnabled 是否启用图像输入：默认开启（对齐 DSH 默认支持图像，仅按模型能力
// 决定是否接受），DSC_NO_VISION=1 可显式关闭（用于不接收图片的模型）。
func visionEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DSC_NO_VISION"))) {
	case "1", "true", "on", "yes":
		return false
	}
	return true
}

// isDeepSeekEndpoint 按 base URL 是否指向 DeepSeek 判定 Files API 可用性
// （llamacpp 等本地 server 无 Files API，始终内联）。
func isDeepSeekEndpoint(baseURL string) bool {
	return strings.Contains(baseURL, "deepseek.com")
}

// toOpenAIMessages 把 core.Message 转换为 OpenAI 请求消息。用户消息携带图像且
// 视觉开启时，构造多模态 content（文本 + image_url / file 块）；assistant 消息
// 回带工具调用。
func (p *OpenAIProvider) toOpenAIMessages(messages []core.Message) []openai.ChatCompletionMessage {
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		msg := openai.ChatCompletionMessage{Role: m.Role}
		if m.Role == "user" && p.vision && len(m.Images) > 0 {
			msg.MultiContent = p.imageContentParts(m.Content, m.Images)
		} else {
			msg.Content = m.Content
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
	return openaiMessages
}

// imageContentParts 构造用户消息的多模态 content：文本块 + 每张图像的块。
// 图像引用（dsc-img:// 或 data URL）先解析为 base64 data URL；单图解码后不超过
// 内联上限时用 image_url，超限且 DeepSeek Files API 可用时自动上传并以 file 块
// 引用 file_id（避免请求体超限）。
func (p *OpenAIProvider) imageContentParts(text string, images []string) []openai.ChatMessagePart {
	parts := make([]openai.ChatMessagePart, 0, len(images)+1)
	if text != "" {
		parts = append(parts, openai.ChatMessagePart{Type: openai.ChatMessagePartTypeText, Text: text})
	}
	for _, img := range images {
		url, err := core.ResolveImageRef(img)
		if err != nil {
			log.Printf("⚠️ 忽略无法解析的图像引用: %v", err)
			continue
		}
		part := openai.ChatMessagePart{
			Type:     openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{URL: url},
		}
		if p.filesAPI && dataURLSize(url) > maxInlineImageBytes {
			if fileID := p.uploadImage(url); fileID != "" {
				part = openai.ChatMessagePart{
					Type: openai.ChatMessagePartTypeFile,
					File: &openai.ChatMessageFile{FileID: fileID},
				}
			}
		}
		parts = append(parts, part)
	}
	return parts
}

// dataURLSize 返回 data URL 解码后的近似字节数（按 base64 长度估算；非 data URL 返回 0）。
func dataURLSize(url string) int {
	const marker = ";base64,"
	i := strings.Index(url, marker)
	if i < 0 {
		return 0
	}
	return len(url[i+len(marker):]) / 4 * 3
}

// parseDataURL 解析 data:image/<mime>;base64,<data> 为 (mime, 原始字节)。
func parseDataURL(url string) (string, []byte, error) {
	i := strings.Index(url, ";base64,")
	if !strings.HasPrefix(url, "data:") || i < 0 {
		return "", nil, os.ErrInvalid
	}
	mime := url[len("data:"):i]
	raw, err := base64.StdEncoding.DecodeString(url[i+len(";base64,"):])
	if err != nil {
		return "", nil, err
	}
	return mime, raw, nil
}

// mimeToExt 由图片 MIME 推导文件扩展名（Files API 上传文件名用）。
func mimeToExt(mime string) string {
	switch strings.ToLower(mime) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	return ".img"
}

// uploadImage 把超大图上传到 DeepSeek Files API（purpose=user_data）并返回 file_id。
// 同一 data URL 只上传一次（进程内缓存，多轮复用）；上传失败返回空串（回退内联）。
func (p *OpenAIProvider) uploadImage(dataURL string) string {
	p.fileMu.Lock()
	defer p.fileMu.Unlock()
	if id, ok := p.fileCache[dataURL]; ok {
		return id
	}
	mime, raw, err := parseDataURL(dataURL)
	if err != nil {
		return ""
	}
	file, err := p.client.CreateFileBytes(context.Background(), openai.FileBytesRequest{
		Name:    "image" + mimeToExt(mime),
		Bytes:   raw,
		Purpose: openai.PurposeUserData,
	})
	if err != nil {
		log.Printf("⚠️ 图片上传 Files API 失败，回退内联: %v", err)
		return ""
	}
	p.fileCache[dataURL] = file.ID
	return file.ID
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []core.Message, tools []core.Tool, maxTokens int) (*core.ChatResponse, error) {
	// 转换消息格式
	openaiMessages := p.toOpenAIMessages(messages)

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
func (p *OpenAIProvider) Version(ctx context.Context) string    { return "1.1.0" } // 支持图像输入（视觉）
func (p *OpenAIProvider) HealthCheck(ctx context.Context) error { return nil }

// ChatStream 實現 LLMProvider.ChatStream 接口
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan *core.ChatStreamResponse, error) {
	// 轉換消息格式
	openaiMessages := p.toOpenAIMessages(messages)

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
		client:    openai.NewClientWithConfig(config),
		model:     model,
		vision:    visionEnabled(),
		filesAPI:  isDeepSeekEndpoint(baseURL),
		fileCache: map[string]string{},
	}

	// 以公共 SDK（dsc-sdk）声明式启动：SDK 复用宿主 core.LLMGRPCPlugin
	// 自动提供 LLMService + 元数据（重写自旧的 plugin.Serve 样板）。
	sdk := dsc.New(dsc.Config{Name: "openai", Version: "1.1.0", Type: dsc.TypeLLM})
	sdk.LLM(provider)
	sdk.Serve()
}
