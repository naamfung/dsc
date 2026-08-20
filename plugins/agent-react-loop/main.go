package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dsc/plugin"
	"dsc/proto"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type ReactLoopAgent struct {
	broker        *goplugin.GRPCBroker
	llmServiceID  uint32
	toolServiceID uint32
	mu            sync.Mutex // 保護 serviceID

	// 快取的服務連接（broker.Dial 為一次性握手，需跨 Run 重用）
	connMu     sync.Mutex
	llmConn    *grpc.ClientConn
	toolConn   *grpc.ClientConn
	llmClient  proto.LLMServiceClient
	toolClient proto.ToolServiceClient

	// 新增字段
	cancelFunc    context.CancelFunc // 用於取消當前 Run
	runWg         sync.WaitGroup     // 等待 Run 完成
	shutdownMu    sync.Mutex         // 保護關閉狀態
	isShutdown    bool

	// 多輪對話記憶：跨 Run 保留會話歷史
	historyMu sync.Mutex
	history   []*proto.Message

	// 上下文容量管理（由宿主透過 DSC_CONTEXT_WINDOW 傳入）
	contextWindow    int        // 總容量（token 數），0 表示未設置（不做自動壓縮）
	lastPromptTokens int32      // 最近一次 LLM 請求的 prompt token 數 ≈ 當前已用容量
	lastUsage        *plugin.Usage // 最近一次 LLM 請求的完整 usage 信息

	// 記錄上次的工具名稱列表，用於檢測模式是否切換
	lastToolNames []string
	// 標記是否需要重置 system prompt（例如模式切換導致工具上下線時）
	sysPromptNeedsUpdate bool

	// 完整 system prompt（基礎指令 + 各工具插件貢獻的上下文片段，如技能索引），
	// 首次對話時構建一次，上下文壓縮後沿用，避免技能索引丢失
	sysPrompt string

	// 單輪模式（-input 自動化測試入口使用）：代理循環僅執行一次，
	// 完成一輪（含工具調用）後自然結束，方便測試後程序自動退出
	singleTurn bool

	// 正常模式下 agent 循環的輪數上限；0 = 不設限（默認，面向長程任務）。
	// 僅當宿主通過 DSC_MAX_ITERATIONS 顯式設置大於 0 的值時才作為上限；
	// 模型不調用工具時循環自然結束，設限僅用於需要控制失控場景。
	maxIterations int

	// persona "你是一個…助手" 身份句，由宿主透過 DSC_PRESET_PERSONA 傳入（預設可配）；
	// 空則回退 DeepSeek 官方默認
	persona string
}

func (a *ReactLoopAgent) SetLLMServiceID(ctx context.Context, id uint32) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.llmServiceID = id
	return nil
}

func (a *ReactLoopAgent) SetToolServiceID(ctx context.Context, id uint32) error {
	a.mu.Lock()
	a.toolServiceID = id
	llmID := a.llmServiceID
	toolID := a.toolServiceID
	a.mu.Unlock()

	// 當兩個 serviceID 都設好後，立即建立連接（在 broker 的 5 秒超時前）
	if llmID != 0 && toolID != 0 {
		go func() {
			_, _, err := a.ensureConnected(llmID, toolID)
			if err != nil {
				fmt.Printf("[Agent Loop] eager connect failed: %v\n", err)
			} else {
				fmt.Printf("[Agent Loop] eager connect successful\n")
			}
		}()
	}
	return nil
}

func (a *ReactLoopAgent) Run(ctx context.Context, input string) (*plugin.AgentResult, error) {
	return a.runLoop(ctx, input, nil)
}

// RunStream 以流式方式执行循环：LLM 文本增量、工具调用提示以帧的形式发送到通道，关闭表示结束
func (a *ReactLoopAgent) RunStream(ctx context.Context, input string) (<-chan *plugin.RunStreamResponse, error) {
	ch := make(chan *plugin.RunStreamResponse)
	go func() {
		defer close(ch)
		var emittedErr bool
		_, err := a.runLoop(ctx, input, func(item *plugin.RunStreamResponse) {
			if item.Status == "error" {
				emittedErr = true
			}
			ch <- item
		})
		// runLoop 在早期失败（serviceID 未设置 / 连接失败等）时不会 emit 錯誤幀，
		// 這裡補發一幀，避免錯誤被吞掉導致 TUI 靜默無響應
		if err != nil && !emittedErr {
			ch <- &plugin.RunStreamResponse{Status: "error", Error: err.Error()}
		}
	}()
	return ch, nil
}

// runLoop 是 Agent 的核心循环；emit 非空时输出流式帧（文本增量 / 工具提示 / 结束状态），否则保持非流式
func (a *ReactLoopAgent) runLoop(ctx context.Context, input string, emit func(*plugin.RunStreamResponse)) (*plugin.AgentResult, error) {
	// 创建一个可取消的 context，保存 cancelFunc
	ctx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	// 如果已有 cancelFunc，先取消（防止并发 Run）
	if a.cancelFunc != nil {
		a.cancelFunc()
	}
	a.cancelFunc = cancel
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.cancelFunc = nil
		a.mu.Unlock()
		a.runWg.Done() // 标记 Run 结束
	}()
	a.runWg.Add(1)

	fmt.Printf("[Agent Loop] Starting turn with input: %s\n", input)

	a.mu.Lock()
	llmID := a.llmServiceID
	toolID := a.toolServiceID
	a.mu.Unlock()

	if llmID == 0 || toolID == 0 {
		return nil, fmt.Errorf("service IDs not set, call SetLLMServiceID and SetToolServiceID first")
	}

	// 獲取（或首次建立）LLM 與 Tool 服務連接
	llmClient, toolClient, err := a.ensureConnected(llmID, toolID)
	if err != nil {
		return nil, err
	}

	// 在循环外获取工具列表（一次即可）
	listToolsResp, err := toolClient.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}
	availableTools := listToolsResp.Tools

	// 提取當前工具名稱列表
	currentToolNames := make([]string, len(availableTools))
	for i, t := range availableTools {
		currentToolNames[i] = t.Name
	}
	// 對工具名稱進行排序，確保比較時不因順序差異而誤判
	sort.Strings(currentToolNames)

	// 檢測工具列表是否變化
	sysPromptChanged := false
	if len(a.lastToolNames) == 0 {
		// 首次初始化
		a.lastToolNames = currentToolNames
	} else {
		// 比較工具列表是否變化（長度不同或內容不同）
		if len(a.lastToolNames) != len(currentToolNames) {
			sysPromptChanged = true
		} else {
			for i, name := range currentToolNames {
				if i >= len(a.lastToolNames) || a.lastToolNames[i] != name {
					sysPromptChanged = true
					break
				}
			}
		}

		if sysPromptChanged {
			a.lastToolNames = currentToolNames
		}
	}

	// 讀取/追加當前用戶輸入到歷史
	a.historyMu.Lock()
	if len(a.history) == 0 {
		// 第一次對話：構建完整 system prompt（基礎指令 + 各插件貢獻的上下文片段，如技能索引）
		a.sysPrompt = a.buildSystemPrompt(ctx, toolClient)
		a.history = []*proto.Message{
			{Role: "system", Content: a.sysPrompt},
		}
	} else {
		// 如果不是第一次對話，且工具列表發生變化（例如模式切換導致工具上下線），則重置 system prompt 並更新歷史中的 system 消息
		if sysPromptChanged {
			a.sysPrompt = a.buildSystemPrompt(ctx, toolClient)
			// 更新歷史中的 system 消息為最新的 sysPrompt
			if len(a.history) > 0 && a.history[0].Role == "system" {
				a.history[0] = &proto.Message{Role: "system", Content: a.sysPrompt}
			}
		}
	}
	a.history = append(a.history, &proto.Message{Role: "user", Content: input})
	currentHistory := a.history
	a.historyMu.Unlock()

	// cancelLoop 返回被取消的结果
	cancelResult := func() (*plugin.AgentResult, error) {
		return &plugin.AgentResult{
			Output: "Agent canceled",
			Status: "error",
		}, ctx.Err()
	}

	// 正常模式沿用 maxIterations（默認 0 = 不設限）；單輪模式（-input）下僅執行一輪，方便測試後自然退出
	maxIterations := a.maxIterations
	if a.singleTurn {
		maxIterations = 1
	}
	executedTools := false     // 記錄本輪是否執行了工具（單輪模式下視為正常完成）
	executedToolsErr := false  // 記錄本輪執行的工具是否出現錯誤（單輪模式下據此判定退出碼）
	// maxIterations == 0 表示不設上限，僅靠 ctx 取消（用戶 Ctrl+C / Shutdown）與模型自然收尾結束
	for i := 0; maxIterations == 0 || i < maxIterations; i++ {
		// 检查 ctx.Done()
		select {
		case <-ctx.Done():
			return cancelResult()
		default:
		}

		// 上下文自動壓縮：已用容量（最近一次請求的 prompt token 數）超過 80% 時，
		// 先讓模型把歷史壓縮成摘要，再繼續後續請求（保持簡單的 80% 觸發邏輯）
		if a.contextWindow > 0 && a.lastPromptTokens > 0 &&
			int(a.lastPromptTokens) >= a.contextWindow*8/10 {
			if emit != nil {
				emit(&plugin.RunStreamResponse{
					Output: fmt.Sprintf("\n[上下文压缩: 已用 %d%% 容量，即将压缩对话历史]\n",
						a.lastPromptTokens*100/int32(a.contextWindow)),
					Status: "tool",
				})
			}
			if err := a.compactHistory(ctx, llmClient, &currentHistory, availableTools); err != nil {
				if emit != nil {
					emit(&plugin.RunStreamResponse{Status: "error", Error: err.Error()})
				}
				return nil, err
			}
			// 壓縮後重新取得工具列表可能無變化，直接繼續下一輪
		}

		// 上下文管理與截斷
		const maxMessages = 20
		if len(currentHistory) > maxMessages {
			// 保留 system 和 user 第一条，删除中间，保留最后几条
			// 简单实现：保留前 2 条（system + initial user）和最后 (maxMessages-2) 条
			kept := currentHistory[:2]
			tailStart := len(currentHistory) - (maxMessages - 2)
			if tailStart < 2 {
				tailStart = 2
			}
			currentHistory = append(kept, currentHistory[tailStart:]...)
		}

		// 调用 LLM（流式或非流式）
		req := &proto.ChatRequest{
			Messages: currentHistory,
			Tools:    availableTools,
		}
		var content string
		var toolCalls []*proto.ToolCall
		if emit == nil {
			resp, err := llmClient.Chat(ctx, req)
			if err != nil {
				return nil, fmt.Errorf("LLM chat failed: %w", err)
			}
			content = resp.Content
			toolCalls = resp.ToolCalls
		} else {
			s, err := llmClient.ChatStream(ctx, req)
			if err != nil {
				emit(&plugin.RunStreamResponse{Status: "error", Error: err.Error()})
				return nil, fmt.Errorf("LLM chat stream failed: %w", err)
			}
			for {
				cr, err := s.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					emit(&plugin.RunStreamResponse{Status: "error", Error: err.Error()})
					return nil, fmt.Errorf("LLM chat stream recv failed: %w", err)
				}
				if cr.Error != "" {
					emit(&plugin.RunStreamResponse{Status: "error", Error: cr.Error})
					return nil, fmt.Errorf("LLM stream error: %s", cr.Error)
				}
				// [DEBUG] 打印接收到的 ChatStreamResponse
				fmt.Fprintf(os.Stderr, "[REACT-LOOP-DEBUG] Frame: Content=%q, Reasoning=%q, FinishReason=%q, Error=%q\n", cr.Content, cr.Reasoning, cr.FinishReason, cr.Error)
				// 記錄 prompt 用量（≈ 當前上下文已用容量）；該值在 finish 分片由服務端返回
				if cr.Usage != nil {
					a.lastPromptTokens = cr.Usage.PromptTokens
					a.lastUsage = plugin.UsageFromProto(cr.Usage)
				}
				content += cr.Content
				if cr.Content != "" {
					emit(&plugin.RunStreamResponse{Output: cr.Content, Status: "streaming"})
				}
				if cr.Reasoning != "" {
					emit(&plugin.RunStreamResponse{Reasoning: cr.Reasoning, Status: "reasoning"})
				}
				if len(cr.ToolCalls) > 0 {
					toolCalls = cr.ToolCalls
				}
			}
		}

		// 追加助手消息（包含文本回复及工具调用；OpenAI 格式要求 assistant 回带 tool_calls）
		assistantMsg := &proto.Message{
			Role:    "assistant",
			Content: content,
		}
		if len(toolCalls) > 0 {
			assistantMsg.ToolCalls = toolCalls
		}
		currentHistory = append(currentHistory, assistantMsg)

		// 没有工具调用 → 返回最终结果（先持久化歷史）
		if len(toolCalls) == 0 {
			a.saveHistory(currentHistory)
			if emit != nil {
				// success 幀攜帶當前已用容量，供 TUI 標題欄顯示「已用/總容量」
				usage := &plugin.Usage{
					PromptTokens: a.lastPromptTokens,
				}
				if a.lastUsage != nil {
					usage.CompletionTokens = a.lastUsage.CompletionTokens
					usage.TotalTokens = a.lastUsage.TotalTokens
				} else {
					usage.TotalTokens = a.lastPromptTokens
				}
				emit(&plugin.RunStreamResponse{
					Status: "success",
					Usage:  usage,
				})
			}
			return &plugin.AgentResult{
				Output: content,
				Status: "success",
			}, nil
		}

		// 执行每个工具并追加结果
		for _, tc := range toolCalls {
			// 检查 ctx.Done()
			select {
			case <-ctx.Done():
				return cancelResult()
			default:
			}

			// 标记本轮执行了工具（单轮模式下据此判定为正常完成）
			executedTools = true

			// 向客户端提示正在调用工具（携带工具名与参数 JSON，供 TUI 渲染 REX 式卡片）
			if emit != nil {
				emit(&plugin.RunStreamResponse{Output: fmt.Sprintf("\n[调用工具: %s]\n", tc.Name), Status: "tool", ToolName: tc.Name, ToolArgs: tc.ArgumentsJson})
			}

			// 执行工具
			toolReq := &proto.ExecuteToolRequest{
				ToolName:      tc.Name,
				ArgumentsJson: tc.ArgumentsJson,
				ToolCallId:    tc.Id,
			}
			toolResp, err := toolClient.ExecuteTool(ctx, toolReq)
			if err != nil {
				// 错误也追加为 tool 消息
				currentHistory = append(currentHistory, &proto.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("Error executing tool %s: %v", tc.Name, err),
					ToolCallId: tc.Id,
				})
				continue
			}
			if toolResp.Error != "" {
				currentHistory = append(currentHistory, &proto.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("Tool error: %s", toolResp.Error),
					ToolCallId: tc.Id,
				})
				// 單輪模式下，工具報錯要視為失敗退出（影響退出碼）
				if a.singleTurn {
					executedToolsErr = true
				}
				// 把工具結果輸出到流，供 TUI 渲染 REX 式结果卡片
				if emit != nil {
					emit(&plugin.RunStreamResponse{Output: fmt.Sprintf("\n[工具结果: %s 错误] %s\n", tc.Name, toolResp.Error), Status: "tool", ToolName: tc.Name, ToolResult: toolResp.Error, Error: toolResp.Error})
				}
			} else {
				currentHistory = append(currentHistory, &proto.Message{
					Role:       "tool",
					Content:    toolResp.Content,
					ToolCallId: tc.Id,
				})
				// 把工具結果輸出到流，供 TUI 渲染 REX 式结果卡片
				if emit != nil {
					emit(&plugin.RunStreamResponse{Output: fmt.Sprintf("\n[工具结果: %s]\n%s\n", tc.Name, toolResp.Content), Status: "tool", ToolName: tc.Name, ToolResult: toolResp.Content})
				}
			}
		}
	}
	// 超过最大迭代次数（持久化歷史）
	a.saveHistory(currentHistory)
	// 單輪模式（-input）下，工具已在本輪執行完畢，視為正常完成（而非“達迭代上限”錯誤），
	// 這樣一次工具調用測試成功後程序能以退出碼 0 自然結束；若工具報錯則以退出碼 1 結束
	if a.singleTurn && executedTools {
		if executedToolsErr {
			// 工具執行出錯：發 error 幀，使 -input 以退出碼 1 結束，測試方能據此判斷失敗
			if emit != nil {
				emit(&plugin.RunStreamResponse{Output: "Single turn completed with tool errors", Status: "error"})
			}
			return &plugin.AgentResult{
				Output: "Single turn completed with tool errors",
				Status: "error",
			}, nil
		}
		if emit != nil {
			emit(&plugin.RunStreamResponse{Status: "success"})
		}
		return &plugin.AgentResult{
			Output: "Single turn completed with tools executed",
			Status: "success",
		}, nil
	}
	if emit != nil {
		// Error 字段必須帶上，否則 TUI 只按 Status 判斷、看不到任何提示，造成「莫名停止」
		emit(&plugin.RunStreamResponse{Output: "Max iterations reached", Status: "error", Error: "达到最大轮次上限，自动停止"})
	}
	return &plugin.AgentResult{
		Output: "Max iterations reached",
		Status: "error",
	}, nil
}

// buildSystemPrompt 構建完整 system prompt，采用 DSH 的分层结构：
// 身份首行 + persona（預設可配）+ 工具指引 + 各工具插件貢獻的上下文片段（如技能索引）。
// 各層按 DSH 约定以空行 "\n\n" 拼接，空內容直接跳過。
func (a *ReactLoopAgent) buildSystemPrompt(ctx context.Context, toolClient proto.ToolServiceClient) string {
	parts := []string{
		// 身份首行（DSH 风格总开关，品牌名适配为 DSC）
		"You are an AI agent powered by DSC.",
	}
	// persona "你是一個…助手" 身份句：预设可配，空則回退 DeepSeek 官方默認
	if p := strings.TrimSpace(a.persona); p != "" {
		parts = append(parts, p)
	} else {
		parts = append(parts, "You are a helpful software engineer assistant.")
	}
	// 工具指引（DSC 是工具型 agent，需提示模型按任务选用工具）
	parts = append(parts, "根据任务选择合适的工具，逐步完成用户的请求。")

	// 聚合各工具插件貢獻的上下文片段（如技能索引），失敗或為空則跳過
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	resp, err := toolClient.ListContext(ctx, &proto.ListContextRequest{})
	cancel()
	if err == nil {
		if content := strings.TrimSpace(resp.GetContent()); content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "\n\n")
}

// saveHistory 將當前的會話上下文寫回 agent 歷史，供下一輪 Run 使用
func (a *ReactLoopAgent) saveHistory(messages []*proto.Message) {
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	a.history = messages
}

// compactSystemPrompt 上下文壓縮指令：要求模型只輸出精簡摘要，不添加額外解釋。
const compactSystemPrompt = "你是对话压缩器。请将下面的对话历史压缩成一段精简但信息完整的摘要，" +
	"保留用户意图、已执行的工具调用及其结果、以及所有关键的中间结论，以便在后续对话中无需原始记录也能继续。" +
	"只输出压缩后的摘要，不要输出任何解释、前言或结尾。"

// compactHistory 當上下文已用容量超過 80% 時觸發：讓模型把 history 壓縮成摘要。
// max_tokens 設為淨餘（contextWindow - lastPromptTokens）的真實值，保證壓縮結果能放入剩餘空間。
func (a *ReactLoopAgent) compactHistory(ctx context.Context, llmClient proto.LLMServiceClient, history *[]*proto.Message, tools []*proto.Tool) error {
	if a.contextWindow <= 0 || a.lastPromptTokens <= 0 {
		return nil
	}
	remaining := a.contextWindow - int(a.lastPromptTokens)
	if remaining < 1024 {
		remaining = 1024
	}
	// 壓縮請求：以 system 指令引導 + 完整歷史作為 user 內容
	// （跳過歷史中的 system 消息，避免把基礎指令與技能索引壓進摘要）
	msgs := make([]*proto.Message, 0, len(*history)+2)
	msgs = append(msgs, &proto.Message{Role: "system", Content: compactSystemPrompt})
	msgs = append(msgs, &proto.Message{Role: "user", Content: "以下是需要压缩的对话历史："})
	for _, m := range *history {
		if m.Role == "system" {
			continue
		}
		msgs = append(msgs, m)
	}

	req := &proto.ChatRequest{
		Messages:  msgs,
		Tools:     tools,
		MaxTokens: int32(remaining),
	}
	resp, err := llmClient.Chat(ctx, req)
	if err != nil {
		return fmt.Errorf("compact history failed: %w", err)
	}
	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return fmt.Errorf("compact history returned empty summary")
	}

	// 用壓縮後的摘要替換歷史（保留完整 system prompt 含技能索引，摘要作為 user 消息）
	sysPrompt := a.sysPrompt
	if sysPrompt == "" {
		sysPrompt = "You are a helpful assistant with access to tools."
	}
	*history = []*proto.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: "以下是此前对话的压缩摘要，请基于它继续当前任务：\n" + summary},
	}
	// 壓縮後已用容量無法從 Chat 精確獲取，重置為 0，下一輪流式調用會更新為真實值
	a.lastPromptTokens = 0
	return nil
}

// ensureConnected 建立並快取 LLM/Tool 服務連接。
// broker.Dial 對同一 serviceID 是一次性握手，連接信息只會發送一次，
// 因此必須跨 Run 重用連接，否則第二次 Dial 會因收不到連接信息而超時。
func (a *ReactLoopAgent) ensureConnected(llmID, toolID uint32) (proto.LLMServiceClient, proto.ToolServiceClient, error) {
	a.connMu.Lock()
	defer a.connMu.Unlock()

	if a.llmClient == nil || a.toolClient == nil {
		llmConn, err := a.broker.Dial(llmID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to dial LLM service: %w", err)
		}
		a.llmConn = llmConn
		a.llmClient = proto.NewLLMServiceClient(llmConn)

		toolConn, err := a.broker.Dial(toolID)
		if err != nil {
			_ = a.llmConn.Close()
			a.llmConn = nil
			a.llmClient = nil
			return nil, nil, fmt.Errorf("failed to dial tool service: %w", err)
		}
		a.toolConn = toolConn
		a.toolClient = proto.NewToolServiceClient(toolConn)
	}
	return a.llmClient, a.toolClient, nil
}

func (a *ReactLoopAgent) Name(ctx context.Context) string { return "react-agent" }
func (a *ReactLoopAgent) Version(ctx context.Context) string { return "1.0.0" }

func (a *ReactLoopAgent) Shutdown(ctx context.Context, force bool) error {
	a.shutdownMu.Lock()
	defer a.shutdownMu.Unlock()

	if a.isShutdown {
		return nil
	}

	// 1. 取消當前 Run
	a.mu.Lock()
	if a.cancelFunc != nil {
		a.cancelFunc()
	}
	a.mu.Unlock()

	// 2. 等待 Run 完成（非強制模式）
	if !force {
		done := make(chan struct{})
		go func() {
			a.runWg.Wait()
			close(done)
		}()
		select {
		case <-done:
			// 正常完成
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second): // 超時保護
			// 超時後強制繼續
		}
	}

	a.isShutdown = true

	// 關閉快取的服務連接
	a.connMu.Lock()
	if a.llmConn != nil {
		_ = a.llmConn.Close()
		a.llmConn = nil
		a.llmClient = nil
	}
	if a.toolConn != nil {
		_ = a.toolConn.Close()
		a.toolConn = nil
		a.toolClient = nil
	}
	a.connMu.Unlock()

	return nil
}

type customAgentPlugin struct {
	goplugin.Plugin
}

func (p *customAgentPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	agent := &ReactLoopAgent{
		broker: broker,
	}
	// 讀取宿主傳入的上下文窗口容量（DSC_CONTEXT_WINDOW，token 數）
	if v := os.Getenv("DSC_CONTEXT_WINDOW"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			agent.contextWindow = n
		}
	}
	// 讀取宿主傳入的單輪模式標記（DSC_SINGLE_TURN=1，-input 自動化測試入口使用）
	if v := os.Getenv("DSC_SINGLE_TURN"); v == "1" || strings.EqualFold(v, "true") {
		agent.singleTurn = true
	}
	// 讀取宿主傳入的循環輪數上限（DSC_MAX_ITERATIONS）。默認 0 = 不設限（面向長程任務），
	// 僅當顯式設置大於 0 的值時才作為循環上限。
	agent.maxIterations = 0
	if v := os.Getenv("DSC_MAX_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			agent.maxIterations = n
		}
	}
	// 讀取宿主傳入的 preset persona（DSC_PRESET_PERSONA，「你是一個…助手」身份句）
	agent.persona = os.Getenv("DSC_PRESET_PERSONA")
	proto.RegisterAgentServiceServer(s, &agentGRPCServer{impl: agent})
	return nil
}

func (p *customAgentPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}

type agentGRPCServer struct {
	proto.UnimplementedAgentServiceServer
	impl plugin.Agent
}

func (s *agentGRPCServer) Run(ctx context.Context, req *proto.RunRequest) (*proto.RunResponse, error) {
	result, err := s.impl.Run(ctx, req.Input)
	if err != nil {
		return nil, err
	}
	return &proto.RunResponse{
		Output: result.Output,
		Status: result.Status,
	}, nil
}

func (s *agentGRPCServer) Name(ctx context.Context, req *proto.NameRequest) (*proto.NameResponse, error) {
	return &proto.NameResponse{Name: s.impl.Name(ctx)}, nil
}

func (s *agentGRPCServer) Version(ctx context.Context, req *proto.VersionRequest) (*proto.VersionResponse, error) {
	return &proto.VersionResponse{Version: s.impl.Version(ctx)}, nil
}

func (s *agentGRPCServer) SetLLMServiceID(ctx context.Context, req *proto.SetLLMServiceIDRequest) (*proto.SetLLMServiceIDResponse, error) {
	err := s.impl.SetLLMServiceID(ctx, req.ServiceId)
	return &proto.SetLLMServiceIDResponse{}, err
}

func (s *agentGRPCServer) SetToolServiceID(ctx context.Context, req *proto.SetToolServiceIDRequest) (*proto.SetToolServiceIDResponse, error) {
	err := s.impl.SetToolServiceID(ctx, req.ServiceId)
	return &proto.SetToolServiceIDResponse{}, err
}

func (s *agentGRPCServer) Shutdown(ctx context.Context, req *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
	err := s.impl.Shutdown(ctx, req.Force)
	if err != nil {
		return &proto.ShutdownResponse{Success: false, Message: err.Error()}, err
	}
	return &proto.ShutdownResponse{Success: true, Message: "shutdown successful"}, nil
}

func (s *agentGRPCServer) RunStream(req *proto.RunRequest, stream proto.AgentService_RunStreamServer) error {
	ch, err := s.impl.RunStream(stream.Context(), req.Input)
	if err != nil {
		return err
	}
	for item := range ch {
		if err := stream.Send(&proto.RunStreamResponse{
			Output:     item.Output,
			Status:     item.Status,
			Error:      item.Error,
			Usage:      plugin.UsageToProto(item.Usage),
			Reasoning:  item.Reasoning,
			ToolName:   item.ToolName,
			ToolArgs:   item.ToolArgs,
			ToolResult: item.ToolResult,
		}); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"agent": &customAgentPlugin{},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}