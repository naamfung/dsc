package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"dsc-sdk"
	"dsc/core"
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
	// vision 是否启用图像输入：默认按模型能力自动判断，DSC_NO_VISION=1 强制关闭。
	vision bool
	// filesAPI 是否可把超大图自动上传 DeepSeek Files API（base URL 为 deepseek.com 时启用）。
	filesAPI bool
	// fileCache 已上传图片的 data URL → file_id 缓存（同一图多轮复用，避免重复上传）。
	fileMu    sync.Mutex
	fileCache map[string]string
	// apiKey/baseURL 供 Files API 上传与消息请求使用（DeepSeek anthropic 端点）。
	apiKey  string
	baseURL string
}

// maxInlineImageBytes 内联 base64 单图大小上限（对齐 DeepSeek 32 MiB 内联限制，
// 预留余量避免请求体逼近 48 MiB 上限；超出则走 Files API 上传）。
const maxInlineImageBytes = 20 << 20 // 20 MiB

// buildMessageParams 构建 Anthropic 请求参数（消息、system、工具定义）。
// maxTokens <= 0 表示使用服务端默认（不再人为限制）；>0 时才显式携带。
// 第二个返回值标记本请求是否引用了 Files API 的 file_id（须带 anthropic-beta 头）。
func (p *AnthropicProvider) buildMessageParams(messages []core.Message, tools []core.Tool, maxTokens int64) (anthropic.MessageNewParams, bool) {
	// 1. 构建 Anthropic 消息
	var systemMsg string
	userMessages := make([]anthropic.MessageParam, 0, len(messages))
	usesBetaHeader := false

	for _, m := range messages {
		switch m.Role {
		case "system":
			systemMsg = m.Content
		case "user":
			if len(m.Images) > 0 {
				// 多模态分支：文本块 + 文件附件块（图像受 p.vision 门控；
				// dsc-txt 文本引用不受视觉限制，始终注入）
				blocks, usesFile := p.fileContentBlocks(m.Content, m.Images)
				if usesFile {
					usesBetaHeader = true
				}
				userMessages = append(userMessages, anthropic.NewUserMessage(blocks...))
			} else {
				userMessages = append(userMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
			}
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
	return msgParams, usesBetaHeader
}

// visionEnabled 是否启用图像输入：默认按模型能力自动判断（服务端在 /models
// 上报 input_modalities 含 image 时启用；未上报则默认放行，对齐 DSH）。
// DSC_NO_VISION=1 可显式强制关闭（自动判断失灵时的逃生口）。
func visionEnabled(baseURL, model string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DSC_NO_VISION"))) {
	case "1", "true", "on", "yes":
		return false
	}
	return core.ModelSupportsImages(baseURL, model)
}

// isDeepSeekEndpoint 按 base URL 是否指向 DeepSeek 判定 Files API 可用性
// （llamacpp 等本地 server 无 Files API，始终内联）。
func isDeepSeekEndpoint(baseURL string) bool {
	return strings.Contains(baseURL, "deepseek.com")
}

// fileContentBlocks 构造用户消息的多模态内容块：文本块 + 文件附件块。
// 文本引用（dsc-txt://）读取内容作为纯文本块注入，不受视觉限制；图像引用
// （dsc-img:// 或 data URL）先解析为 base64 data URL，单图解码后不超过内联上限
// 时用 base64 源、超限且 DeepSeek Files API 可用时自动上传并以 file 源引用 file_id
// （请求需带 anthropic-beta 头）；图像仅当视觉开启（p.vision）。返回内容块与
// 是否使用了 file 源。
func (p *AnthropicProvider) fileContentBlocks(text string, refs []string) ([]anthropic.ContentBlockParamUnion, bool) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(refs)+1)
	usesFile := false
	if text != "" {
		blocks = append(blocks, anthropic.NewTextBlock(text))
	}
	for _, ref := range refs {
		if strings.HasPrefix(ref, core.TextRefPrefix) {
			content, err := core.ResolveTextRef(ref)
			if err != nil {
				log.Printf("⚠️ 忽略无法解析的文本引用: %v", err)
				continue
			}
			blocks = append(blocks, anthropic.NewTextBlock(content))
			continue
		}
		if !p.vision {
			continue // 视觉关闭：跳过图像引用
		}
		url, err := core.ResolveImageRef(ref)
		if err != nil {
			log.Printf("⚠️ 忽略无法解析的图像引用: %v", err)
			continue
		}
		mime, b64, ok := splitDataURL(url)
		if !ok {
			continue
		}
		if p.filesAPI && dataURLSize(url) > maxInlineImageBytes {
			if fileID := p.uploadImage(url); fileID != "" {
				usesFile = true
				blocks = append(blocks, anthropic.NewImageBlock(anthropic.FileImageSourceParam{
					FileID:    fileID,
					MediaType: anthropic.Base64ImageSourceMediaType(mime),
				}))
				continue
			}
		}
		blocks = append(blocks, anthropic.NewImageBlockBase64(mime, b64))
	}
	return blocks, usesFile
}

// splitDataURL 解析 data:image/<mime>;base64,<b64> 为 (mime, base64 负载)。
func splitDataURL(url string) (string, string, bool) {
	i := strings.Index(url, ";base64,")
	if !strings.HasPrefix(url, "data:") || i < 0 {
		return "", "", false
	}
	return url[len("data:"):i], url[i+len(";base64,"):], true
}

// dataURLSize 返回 data URL 解码后的近似字节数（按 base64 长度估算；非 data URL 返回 0）。
func dataURLSize(url string) int {
	_, b64, ok := splitDataURL(url)
	if !ok {
		return 0
	}
	return len(b64) / 4 * 3
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

// uploadImage 把超大图上传到 DeepSeek anthropic 兼容 Files API（purpose=user_data）
// 并返回 file_id。同一 data URL 只上传一次（进程内缓存，多轮复用）；失败返回空串。
func (p *AnthropicProvider) uploadImage(dataURL string) string {
	p.fileMu.Lock()
	defer p.fileMu.Unlock()
	if id, ok := p.fileCache[dataURL]; ok {
		return id
	}
	mime, b64, ok := splitDataURL(dataURL)
	if !ok {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return ""
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("purpose", "user_data")
	part, err := writer.CreateFormFile("file", "image"+mimeToExt(mime))
	if err != nil {
		return ""
	}
	if _, err := part.Write(raw); err != nil {
		return ""
	}
	if err := writer.Close(); err != nil {
		return ""
	}

	endpoint := strings.TrimRight(p.baseURL, "/") + "/v1/files"
	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("anthropic-beta", "files-api-2025-04-14")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("⚠️ 图片上传 Files API 失败: %v", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		log.Printf("⚠️ 图片上传 Files API 失败（HTTP %d）: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
		return ""
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.ID == "" {
		return ""
	}
	p.fileCache[dataURL] = out.ID
	return out.ID
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
func extractToolCalls(blocks []anthropic.ContentBlockUnion) []core.ToolCall {
	var toolCalls []core.ToolCall
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
		toolCalls = append(toolCalls, core.ToolCall{
			ID:        toolUse.ID,
			Name:      toolUse.Name,
			Arguments: argsMap,
		})
	}
	return toolCalls
}

// usageFromAnthropic 将 Anthropic usage 转换为 core.Usage（nil 返回 nil）。
// 兼容启用提示缓存的 llama.cpp 等接口：其 input_tokens 仅统计未命中缓存的新增 token，
// 已命中前缀单独计入 cache_read_input_tokens，真实上下文长度 = input + cache_read；
// 标准 Anthropic 接口的 input_tokens 已含全部输入（cache_read 为其子集），
// 此时 cache_read <= input，保持原值即可避免重复计数。
// 命中率计算需要 miss 侧：以 CacheCreationInputTokens = 真实 prompt - cache_read 近似
// （即本次请求中未被缓存提供的新增 token），否则未命中侧恒为 0，命中率恒显示 100%。
func usageFromAnthropic(u *anthropic.Usage) *core.Usage {
	if u == nil {
		return nil
	}
	promptTokens := u.InputTokens
	if u.CacheReadInputTokens > u.InputTokens {
		promptTokens += u.CacheReadInputTokens
	}
	if promptTokens <= 0 && u.OutputTokens <= 0 {
		return nil
	}
	cacheMiss := promptTokens - u.CacheReadInputTokens
	if cacheMiss < 0 {
		cacheMiss = 0
	}
	return &core.Usage{
		PromptTokens:             int32(promptTokens),
		CompletionTokens:         int32(u.OutputTokens),
		TotalTokens:              int32(promptTokens + u.OutputTokens),
		CacheReadInputTokens:     int32(u.CacheReadInputTokens),
		CacheCreationInputTokens: int32(cacheMiss),
	}
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []core.Message, tools []core.Tool, maxTokens int) (*core.ChatResponse, error) {
	params, beta := p.buildMessageParams(messages, tools, int64(maxTokens))
	opts := p.requestOptions(beta)
	resp, err := p.client.Messages.New(ctx, params, opts...)
	if err != nil {
		return nil, err
	}

	return &core.ChatResponse{
		Content:      concatText(resp.Content),
		FinishReason: string(resp.StopReason),
		ToolCalls:    extractToolCalls(resp.Content),
	}, nil
}

// requestOptions 返回本次消息请求的额外选项：引用 Files API file_id 时附加
// anthropic-beta 头（DeepSeek 要求）。
func (p *AnthropicProvider) requestOptions(beta bool) []option.RequestOption {
	if !beta {
		return nil
	}
	return []option.RequestOption{
		option.WithHeader("anthropic-beta", "files-api-2025-04-14"),
	}
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, messages []core.Message, tools []core.Tool) (<-chan *core.ChatStreamResponse, error) {
	params, beta := p.buildMessageParams(messages, tools, p.maxTokens)
	stream := p.client.Messages.NewStreaming(ctx, params, p.requestOptions(beta)...)

	ch := make(chan *core.ChatStreamResponse)
	go func() {
		defer close(ch)
		acc := newStreamAccumulator()
		prevLen := 0
		prevReasonLen := 0
		for stream.Next() {
			event := stream.Current()
			if err := acc.accumulate(event); err != nil {
				ch <- &core.ChatStreamResponse{Error: fmt.Sprintf("stream accumulate error: %v", err)}
				return
			}
			// 仅当文本有新增时才发送增量帧
			text := concatText(acc.msg.Content)
			if len(text) > prevLen {
				ch <- &core.ChatStreamResponse{Content: text[prevLen:]}
				prevLen = len(text)
			}
			// 思考过程增量（thinking 块）
			reason := concatReasoning(acc.msg.Content)
			if len(reason) > prevReasonLen {
				// [DEBUG] 打印 reasoning 帧
				fmt.Fprintf(os.Stderr, "[LLM-ANTHROPIC-REASONING] %s\n", reason[prevReasonLen:])
				ch <- &core.ChatStreamResponse{Reasoning: reason[prevReasonLen:]}
				prevReasonLen = len(reason)
			}
		}
		if err := stream.Err(); err != nil {
			ch <- &core.ChatStreamResponse{Error: err.Error()}
			return
		}
		// 流结束：发送最终帧（工具调用与结束原因；文本已增量发送，不再重复）
		resp := &core.ChatStreamResponse{
			FinishReason: string(acc.msg.StopReason),
			ToolCalls:    extractToolCalls(acc.msg.Content),
		}
		// 處理 usage 信息（Anthropic 格式：input_tokens -> prompt_tokens, output_tokens -> completion_tokens；
		// cache_read/cache_creation 直接对应）
		resp.Usage = usageFromAnthropic(&acc.msg.Usage)
		ch <- resp
	}()
	return ch, nil
}

func (p *AnthropicProvider) Name(ctx context.Context) string {
	return "anthropic"
}

func (p *AnthropicProvider) Version(ctx context.Context) string {
	return "1.2.0" // 版本升级：支持图像输入（视觉）
}

func (p *AnthropicProvider) VisionEnabled() bool { return p.vision }

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
		vision:         visionEnabled(baseURL, model),
		filesAPI:       isDeepSeekEndpoint(baseURL),
		fileCache:      map[string]string{},
		apiKey:         apiKey,
		baseURL:        baseURL,
	}

	// 以公共 SDK（dsc-sdk）声明式启动：SDK 复用宿主 core.LLMGRPCPlugin
	// 自动提供 LLMService + 元数据（重写自旧的 plugin.Serve 样板）。
	sdk := dsc.New(dsc.Config{Name: "anthropic", Version: "1.2.0", Type: dsc.TypeLLM})
	sdk.LLM(provider)
	sdk.Serve()
}
