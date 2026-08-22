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
	"dsc/session"
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

	// 用户评审通道（宿主挂载的 UserQuestionsService）：exit_plan_mode 等工具
	// 向宿主询问用户并等待回答；serviceID 由宿主经 SetUserQuestionsService 注入。
	uqServiceID uint32
	uqMu        sync.Mutex
	uqConn      *grpc.ClientConn
	uqClient    proto.UserQuestionsServiceClient

	// 新增字段
	cancelFunc context.CancelFunc // 用於取消當前 Run
	runWg      sync.WaitGroup     // 等待 Run 完成
	shutdownMu sync.Mutex         // 保護關閉狀態
	isShutdown bool

	// 多輪對話記憶：跨 Run 保留會話歷史（事件溯源日志）
	sessMu      sync.Mutex
	sess        *session.Session
	turnCounter int // 轮次编号（跨 Run 递增，对齐 DSH 的 turn 概念）

	// 多会话事件日志存储（DSC_SESSION_DIR，缺省 ./sessions）；固定使用 default 会话
	store *session.Store

	// 上下文容量管理（由宿主透過 DSC_CONTEXT_WINDOW 傳入）
	contextWindow    int           // 總容量（token 數），0 表示未設置（不做自動壓縮）
	lastPromptTokens int32         // 最近一次 LLM 請求的 prompt token 數 ≈ 當前已用容量
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

	// plan/goal 宿主工具状态（对齐 DSH plan-mode + goal 领域）：
	// planSection 为 plan 模式激活时注入 system prompt 的部署方引导文案（DSC_PLAN_SECTION，
	// 缺省 DSH 示例文案）；planActive 为当前会话的 plan 模式（每次 Run 从事件日志折叠）。
	// goalActivation 是进程本地续行启用状态（绝不持久化：恢复/fork 后停用，需显式 resume
	// 重新启用）；goalRounds 为已准入 Goal Round 数（v1 恒 0，jobs/workflow 落地后推进）。
	planSection                   string
	defaultMaxGoalRounds          int
	blockedAfterConsecutiveRounds int
	planActive                    bool
	goalActivation                bool
	goalRounds                    int

	// todo 任务清单部署配置（对齐 DSH allowParallelInProgress 开关）：
	// todoAllowParallel 为 true 时允许多个任务同时 in_progress（DSC_TODO_ALLOW_PARALLEL，
	// 缺省 false 强制单活跃项纪律）。
	todoAllowParallel bool
}

func (a *ReactLoopAgent) RegisterServices(ctx context.Context, llmServiceID, toolServiceID uint32) error {
	a.mu.Lock()
	a.llmServiceID = llmServiceID
	a.toolServiceID = toolServiceID
	a.mu.Unlock()

	// 兩個 serviceID 一次性就緒後，立即建立連接（在 broker 的 5 秒超時前）
	if llmServiceID != 0 && toolServiceID != 0 {
		go func() {
			_, _, err := a.ensureConnected(llmServiceID, toolServiceID)
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
		return nil, fmt.Errorf("service IDs not set, call RegisterServices first")
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

	// 事件溯源会话：首轮从磁盘恢复或新建（多会话 Store，固定 default 会话），
	// 并构建完整 system prompt；工具列表变化时重建 system prompt。
	// 历史以事件追加进 session（对齐 DSH：模型可见即已记录），不再维护独立消息数组。
	a.sessMu.Lock()
	if a.sess == nil {
		restored, err := a.store.Ensure("default")
		if err != nil {
			a.sessMu.Unlock()
			return nil, fmt.Errorf("failed to restore session: %w", err)
		}
		if restored.Len() > 0 {
			a.turnCounter = restored.LastTurn()
			fmt.Printf("[Agent Loop] session restored from store (%d events, last turn %d)\n",
				restored.Len(), a.turnCounter)
		}
		a.sess = restored
		a.goalActivation = false // 恢复/切换后停用续行（对齐 DSH session-start disarm）
		a.sysPrompt = a.buildSystemPrompt(ctx, toolClient)
	} else if sysPromptChanged || a.sysPromptNeedsUpdate {
		a.sysPrompt = a.buildSystemPrompt(ctx, toolClient)
		a.sysPromptNeedsUpdate = false
	}
	// plan 模式每次 Run 从事件日志折叠（SetPlanMode/exit_plan_mode 已提交的变更即时生效）
	a.planActive = session.FoldPlanMode(a.sess.Events())
	a.turnCounter++
	turnNo := a.turnCounter
	sess := a.sess
	a.sessMu.Unlock()

	// 宿主托管的工具（plan/goal + ask_user_question + todo_write）追加进模型可见目录（执行时拦截，见下方工具循环）
	availableTools = append(availableTools, a.hostTools()...)

	// 每次 Run 结束（含错误路径）将事件日志落盘（多会话 Store）
	defer func() {
		if err := a.store.Save(sess); err != nil {
			fmt.Printf("[Agent Loop] failed to save session: %v\n", err)
		}
	}()

	// 轮次与用户输入作为会话事件记录（turn/start 为 log-only，user/message 进入 surface）
	sess.Append(session.TurnStart, &session.TurnData{Turn: turnNo}, nil)
	sess.Append(session.UserMessage, &session.UserMessageData{Content: input, Source: "user"}, &session.SurfaceOp{Op: session.SurfaceAppend})

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
	executedTools := false    // 記錄本輪是否執行了工具（單輪模式下視為正常完成）
	executedToolsErr := false // 記錄本輪執行的工具是否出現錯誤（單輪模式下據此判定退出碼）
	stepNo := 0
	// maxIterations == 0 表示不設上限，僅靠 ctx 取消（用戶 Ctrl+C / Shutdown）與模型自然收尾結束
	for i := 0; maxIterations == 0 || i < maxIterations; i++ {
		// 检查 ctx.Done()
		select {
		case <-ctx.Done():
			return cancelResult()
		default:
		}

		stepNo++
		// 步骤边界（log-only）
		sess.Append(session.StepStart, &session.StepData{Turn: turnNo, Step: stepNo}, nil)

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
			summary, err := a.compactHistory(ctx, llmClient, sess.DeriveMessages(a.sysPrompt), availableTools)
			if err != nil {
				if emit != nil {
					emit(&plugin.RunStreamResponse{Status: "error", Error: err.Error()})
				}
				return nil, err
			}
			// 压缩落地（surface replace）：追加 CompactionSummary 事件遮蔽全部旧 surface
			// 节点，派生历史退化为 [system, 摘要]，但原事件保留在日志（append-only 无损，
			// 可回放/恢复）。
			nodes := sess.SurfaceNodes()
			if len(nodes) > 0 {
				sess.Append(session.CompactionSummary, &session.CompactionSummaryData{
					Content: "以下是此前对话的压缩摘要，请基于它继续当前任务：\n" + summary,
				}, &session.SurfaceOp{Op: session.SurfaceReplace, Start: nodes[0], End: nodes[len(nodes)-1]})
			} else {
				sess.Append(session.CompactionSummary, &session.CompactionSummaryData{
					Content: summary,
				}, &session.SurfaceOp{Op: session.SurfaceAppend})
			}
			// 壓縮後已用容量無法從 Chat 精確獲取，重置為 0，下一輪流式調用會更新為真實值
			a.lastPromptTokens = 0
		}

		// 请求历史由会话 surface 派生（system prompt 前置），不再依赖独立消息数组
		msgs := sess.DeriveMessages(a.sysPrompt)

		// 调用 LLM（流式或非流式）
		req := &proto.ChatRequest{
			Messages: msgs,
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
				// 原始流分片记入会话（log-only：回放/UI 保真，不参与派生历史）
				if cr.Content != "" || cr.Reasoning != "" {
					sess.Append(session.AssistantChunk, &session.AssistantChunkData{
						Turn: turnNo, Step: stepNo, Content: cr.Content, Reasoning: cr.Reasoning,
					}, nil)
				}
				if len(cr.ToolCalls) > 0 {
					toolCalls = cr.ToolCalls
				}
			}
		}

		// 组装后的助手消息记入会话（surface；携带 usage 与工具调用，对齐 DSH assistant/message）
		lastUsage := &proto.Usage{}
		if a.lastUsage != nil {
			lastUsage = &proto.Usage{
				PromptTokens:     a.lastUsage.PromptTokens,
				CompletionTokens: a.lastUsage.CompletionTokens,
				TotalTokens:      a.lastUsage.TotalTokens,
			}
		}
		sess.Append(session.AssistantMessage, &session.AssistantMessageData{
			Turn:      turnNo,
			Step:      stepNo,
			Content:   content,
			ToolCalls: toolCalls,
			Usage:     lastUsage,
		}, &session.SurfaceOp{Op: session.SurfaceAppend})

		// 没有工具调用 → 目标续行驱动器检查（对齐 DSH goal-round-driver）：
		// goal active+armed+预算未耗尽时，准入下一轮 goal-round 用户消息并继续循环。
		// 单轮模式（-input 自动化）不自动续行；人类消息不消耗预算。
		if len(toolCalls) == 0 {
			if !a.singleTurn {
				if g := session.FoldGoal(sess.Events()); goalRoundDriver(g, a.goalActivation, a.goalRounds) {
					a.goalRounds++
					sess.Append(session.UserMessage, &session.UserMessageData{
						Content: goalRoundPrompt(g, a.goalRounds),
						Source:  "goal_round",
					}, &session.SurfaceOp{Op: session.SurfaceAppend})
					if emit != nil {
						emit(&plugin.RunStreamResponse{
							Output: fmt.Sprintf("\n[Goal Round %d/%d]\n", a.goalRounds, g.MaxGoalRounds),
							Status: "tool",
						})
					}
					continue
				}
			}
			sess.Append(session.TurnEnd, &session.TurnData{Turn: turnNo, Reason: "completed"}, nil)
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
		concludeTurn := false // 宿主 goal 工具 complete/blocked 后结束物理轮次（对齐 DSH concludeTurn）
		for _, tc := range toolCalls {
			// 检查 ctx.Done()
			select {
			case <-ctx.Done():
				return cancelResult()
			default:
			}

			// 标记本轮执行了工具（单轮模式下据此判定为正常完成）
			executedTools = true

			// 模型请求的工具调用记入会话（log-only，与下方 tool/result 配对）
			sess.Append(session.ToolCallEvent, &session.ToolCallData{
				Turn: turnNo, Step: stepNo, CallID: tc.Id, Name: tc.Name, Arguments: tc.ArgumentsJson,
			}, nil)

			// 向客户端提示正在调用工具（携带工具名与参数 JSON，供 TUI 渲染 REX 式卡片）
			if emit != nil {
				emit(&plugin.RunStreamResponse{Output: fmt.Sprintf("\n[调用工具: %s]\n", tc.Name), Status: "tool", ToolName: tc.Name, ToolArgs: tc.ArgumentsJson})
			}

			// 宿主托管的 plan/goal 工具直接本地执行（状态读写会话事件日志），
			// 其余工具经聚合 ToolService 转发到工具插件
			var toolResp *proto.ExecuteToolResponse
			if isLocalTool(tc.Name) {
				var concluded bool
				toolResp, concluded = a.executeLocalTool(ctx, tc)
				if concluded {
					concludeTurn = true
				}
			} else {
				toolReq := &proto.ExecuteToolRequest{
					ToolName:      tc.Name,
					ArgumentsJson: tc.ArgumentsJson,
					ToolCallId:    tc.Id,
				}
				var err error
				toolResp, err = toolClient.ExecuteTool(ctx, toolReq)
				if err != nil {
					// 错误也作为 tool/result 记入（surface）
					sess.Append(session.ToolResult, &session.ToolResultData{
						Turn: turnNo, Step: stepNo, CallID: tc.Id,
						Content: fmt.Sprintf("Error executing tool %s: %v", tc.Name, err),
						Error:   err.Error(),
					}, &session.SurfaceOp{Op: session.SurfaceAppend})
					continue
				}
			}
			if toolResp.Error != "" {
				sess.Append(session.ToolResult, &session.ToolResultData{
					Turn: turnNo, Step: stepNo, CallID: tc.Id,
					Content: fmt.Sprintf("Tool error: %s", toolResp.Error),
					Error:   toolResp.Error,
				}, &session.SurfaceOp{Op: session.SurfaceAppend})
				// 單輪模式下，工具報錯要視為失敗退出（影響退出碼）
				if a.singleTurn {
					executedToolsErr = true
				}
				// 把工具結果輸出到流，供 TUI 渲染 REX 式结果卡片
				if emit != nil {
					emit(&plugin.RunStreamResponse{Output: fmt.Sprintf("\n[工具结果: %s 错误] %s\n", tc.Name, toolResp.Error), Status: "tool", ToolName: tc.Name, ToolResult: toolResp.Error, Error: toolResp.Error})
				}
			} else {
				sess.Append(session.ToolResult, &session.ToolResultData{
					Turn: turnNo, Step: stepNo, CallID: tc.Id,
					Content: toolResp.Content,
				}, &session.SurfaceOp{Op: session.SurfaceAppend})
				// 把工具結果輸出到流，供 TUI 渲染 REX 式结果卡片
				if emit != nil {
					emit(&plugin.RunStreamResponse{Output: fmt.Sprintf("\n[工具结果: %s]\n%s\n", tc.Name, toolResp.Content), Status: "tool", ToolName: tc.Name, ToolResult: toolResp.Content})
				}
			}
		}
		// 步骤结束（log-only）
		sess.Append(session.StepEnd, &session.StepData{Turn: turnNo, Step: stepNo}, nil)

		// 宿主 goal 工具标记 complete/blocked：物理轮次在本步骤后停止（对齐 DSH concludeTurn）
		if concludeTurn {
			sess.Append(session.TurnEnd, &session.TurnData{Turn: turnNo, Reason: "goal-concluded"}, nil)
			if emit != nil {
				usage := &plugin.Usage{PromptTokens: a.lastPromptTokens}
				if a.lastUsage != nil {
					usage.CompletionTokens = a.lastUsage.CompletionTokens
					usage.TotalTokens = a.lastUsage.TotalTokens
				} else {
					usage.TotalTokens = a.lastPromptTokens
				}
				emit(&plugin.RunStreamResponse{Status: "success", Usage: usage})
			}
			return &plugin.AgentResult{Output: content, Status: "success"}, nil
		}
	}
	// 超过最大迭代次数：轮次以 max-iterations 关闭（不再持久化独立消息数组，session 即历史）
	sess.Append(session.TurnEnd, &session.TurnData{Turn: turnNo, Reason: "max-iterations"}, nil)
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

	// plan 模式引导（仅激活时注入；对齐 DSH plan:policy 段落，软引导不强制任何限制）
	if a.planActive {
		if s := strings.TrimSpace(a.planSection); s != "" {
			parts = append(parts, s)
		}
	}

	// goal 策略指引（对齐 DSH tool-goal 固定提示词段；goal 状态本身不注入模型上下文）
	parts = append(parts, fmt.Sprintf(goalPolicyPrompt, a.blockedAfterConsecutiveRounds))

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

// compactSystemPrompt 上下文壓縮指令：要求模型只輸出精簡摘要，不添加額外解釋。
const compactSystemPrompt = "你是对话压缩器。请将下面的对话历史压缩成一段精简但信息完整的摘要，" +
	"保留用户意图、已执行的工具调用及其结果、以及所有关键的中间结论，以便在后续对话中无需原始记录也能继续。" +
	"只输出压缩后的摘要，不要输出任何解释、前言或结尾。"

// compactHistory 當上下文已用容量超過 80% 時觸發：讓模型把派生歷史壓縮成摘要。
// 返回摘要文本（不含 system 消息，避免把基礎指令與技能索引壓進摘要），
// 由調用方決定落點（重建會話或 surface replace 遮蔽）。
// max_tokens 設為淨餘（contextWindow - lastPromptTokens）的真實值，保證壓縮結果能放入剩餘空間。
func (a *ReactLoopAgent) compactHistory(ctx context.Context, llmClient proto.LLMServiceClient, msgs []*proto.Message, tools []*proto.Tool) (string, error) {
	if a.contextWindow <= 0 || a.lastPromptTokens <= 0 {
		return "", nil
	}
	remaining := a.contextWindow - int(a.lastPromptTokens)
	if remaining < 1024 {
		remaining = 1024
	}
	// 壓縮請求：以 system 指令引導 + 派生歷史作為 user 內容
	// （跳過歷史中的 system 消息，避免把基礎指令與技能索引壓進摘要）
	reqMsgs := make([]*proto.Message, 0, len(msgs)+2)
	reqMsgs = append(reqMsgs, &proto.Message{Role: "system", Content: compactSystemPrompt})
	reqMsgs = append(reqMsgs, &proto.Message{Role: "user", Content: "以下是需要压缩的对话历史："})
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		reqMsgs = append(reqMsgs, m)
	}

	req := &proto.ChatRequest{
		Messages:  reqMsgs,
		Tools:     tools,
		MaxTokens: int32(remaining),
	}
	resp, err := llmClient.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("compact history failed: %w", err)
	}
	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return "", fmt.Errorf("compact history returned empty summary")
	}
	return summary, nil
}

// SetUserQuestionsService 注入宿主挂载在 broker 上的 UserQuestionsService ID。
// exit_plan_mode 评审等场景经 ensureUserQuestionsClient 惰性连接并调用。
func (a *ReactLoopAgent) SetUserQuestionsService(ctx context.Context, serviceID uint32) error {
	a.uqMu.Lock()
	defer a.uqMu.Unlock()
	a.uqServiceID = serviceID
	fmt.Printf("[Agent Loop] user-questions service set (id=%d)\n", serviceID)
	return nil
}

// ensureUserQuestionsClient 惰性建立 UserQuestionsService 连接（返回 nil 表示无通道）。
func (a *ReactLoopAgent) ensureUserQuestionsClient() proto.UserQuestionsServiceClient {
	a.uqMu.Lock()
	defer a.uqMu.Unlock()
	if a.uqServiceID == 0 {
		return nil
	}
	if a.uqClient != nil {
		return a.uqClient
	}
	conn, err := a.broker.Dial(a.uqServiceID)
	if err != nil {
		fmt.Printf("[Agent Loop] dial user-questions service failed: %v\n", err)
		return nil
	}
	a.uqConn = conn
	a.uqClient = proto.NewUserQuestionsServiceClient(conn)
	return a.uqClient
}

// SwitchSession 切换当前会话：从 store 按 id 加载并接管（事件溯源日志）。
// 下一次 Run 将基于目标会话继续；当前轮次若在进行中由调用方负责确保已结束。
func (a *ReactLoopAgent) SwitchSession(ctx context.Context, sessionID string) error {
	if a.store == nil {
		return fmt.Errorf("session store not initialized")
	}
	sess, err := a.store.Ensure(sessionID)
	if err != nil {
		return fmt.Errorf("switch session: %w", err)
	}
	a.sessMu.Lock()
	a.sess = sess
	a.turnCounter = sess.LastTurn()
	a.sysPromptNeedsUpdate = true // 新会话的 plan/goal 状态不同，下次 Run 重建 system prompt
	a.sessMu.Unlock()
	fmt.Printf("[Agent Loop] switched to session %s (%d events, last turn %d)\n",
		sessionID, sess.Len(), a.turnCounter)
	return nil
}

// SetPlanMode 设置当前会话的 plan 模式：追加 log-only plan/mode 事件并落盘
// （事件溯源：恢复/fork/压缩都能直接折叠回 plan 状态），并标记下次 Run 重建
// system prompt 以注入/移除 plan section。对齐 DSH plan-mode 的软引导设计：
// 仅注入引导文案，不强制任何沙箱或批准限制。
func (a *ReactLoopAgent) SetPlanMode(ctx context.Context, active bool) error {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	if a.sess == nil {
		return fmt.Errorf("set plan mode: session not loaded")
	}
	a.sess.Append(session.PlanMode, &session.PlanModeData{Active: active}, nil)
	if err := a.store.Save(a.sess); err != nil {
		return fmt.Errorf("set plan mode: %w", err)
	}
	a.sysPromptNeedsUpdate = true
	fmt.Printf("[Agent Loop] plan mode set to %v\n", active)
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

func (a *ReactLoopAgent) Name(ctx context.Context) string    { return "react-agent" }
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
	a.uqMu.Lock()
	if a.uqConn != nil {
		_ = a.uqConn.Close()
		a.uqConn = nil
		a.uqClient = nil
	}
	a.uqMu.Unlock()
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
	// plan 模式引导文案（DSC_PLAN_SECTION，缺省 DSH 示例文案）
	agent.planSection = os.Getenv("DSC_PLAN_SECTION")
	if agent.planSection == "" {
		agent.planSection = defaultPlanSection
	}
	// goal 部署默认 Round 上限（DSC_GOAL_MAX_ROUNDS，缺省 256 对齐 DSH）
	agent.defaultMaxGoalRounds = 256
	if v := os.Getenv("DSC_GOAL_MAX_ROUNDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			agent.defaultMaxGoalRounds = n
		}
	}
	// goal 阻塞判定阈值（DSC_GOAL_BLOCKED_AFTER，缺省 3 对齐 DSH；写入策略提示词）
	agent.blockedAfterConsecutiveRounds = 3
	if v := os.Getenv("DSC_GOAL_BLOCKED_AFTER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			agent.blockedAfterConsecutiveRounds = n
		}
	}
	// todo 任务清单并行开关（DSC_TODO_ALLOW_PARALLEL，缺省 false：最多一个 in_progress）
	if v := os.Getenv("DSC_TODO_ALLOW_PARALLEL"); v == "1" || strings.EqualFold(v, "true") {
		agent.todoAllowParallel = true
	}
	// 多会话事件日志存储（DSC_SESSION_DIR，缺省落在插件工作目录下的 sessions/）
	dir := os.Getenv("DSC_SESSION_DIR")
	if dir == "" {
		dir = "sessions"
	}
	store, err := session.NewStore(dir)
	if err != nil {
		return fmt.Errorf("failed to open session store: %w", err)
	}
	agent.store = store
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

func (s *agentGRPCServer) RegisterServices(ctx context.Context, req *proto.RegisterServicesRequest) (*proto.RegisterServicesResponse, error) {
	err := s.impl.RegisterServices(ctx, req.LlmServiceId, req.ToolServiceId)
	return &proto.RegisterServicesResponse{}, err
}

func (s *agentGRPCServer) SwitchSession(ctx context.Context, req *proto.SwitchSessionRequest) (*proto.SwitchSessionResponse, error) {
	if err := s.impl.SwitchSession(ctx, req.SessionId); err != nil {
		return &proto.SwitchSessionResponse{Success: false, Message: err.Error()}, nil
	}
	return &proto.SwitchSessionResponse{Success: true}, nil
}

func (s *agentGRPCServer) SetPlanMode(ctx context.Context, req *proto.SetPlanModeRequest) (*proto.SetPlanModeResponse, error) {
	if err := s.impl.SetPlanMode(ctx, req.Active); err != nil {
		return &proto.SetPlanModeResponse{Success: false, Message: err.Error()}, nil
	}
	return &proto.SetPlanModeResponse{Success: true}, nil
}

func (s *agentGRPCServer) SetUserQuestionsService(ctx context.Context, req *proto.SetUserQuestionsServiceRequest) (*proto.SetUserQuestionsServiceResponse, error) {
	if err := s.impl.SetUserQuestionsService(ctx, req.ServiceId); err != nil {
		return &proto.SetUserQuestionsServiceResponse{Success: false, Message: err.Error()}, nil
	}
	return &proto.SetUserQuestionsServiceResponse{Success: true}, nil
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
