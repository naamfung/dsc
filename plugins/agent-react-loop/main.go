package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"dsc-sdk"
	"dsc/core"
	"dsc/proto"
	"dsc/session"
	"google.golang.org/grpc"
)

type ReactLoopAgent struct {
	broker        *dsc.AgentBroker // SDK 隔离封装的宿主 broker（Dial 宿主 LLM/Tool/UserQuestions）
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

	// 运行中注入计数：TUI 在模型输出期间经 InjectMessage 实时注入的新用户消息数
	// （sessMu 保护）。runLoop 每轮派生请求历史时快照消费；模型无工具调用收尾前若发现
	// 快照后有新注入，说明存在尚未发送给模型的新消息，须继续下一轮而不是结束本轮。
	pendingInjects int

	// 多会话事件日志存储（DSC_SESSION_DIR，缺省 ./sessions）；固定使用 default 会话
	store *session.Store

	// 上下文容量管理（由宿主透過 DSC_CONTEXT_WINDOW 傳入）
	contextWindow    int         // 總容量（token 數），0 表示未設置（不做自動壓縮）
	lastPromptTokens int32       // 最近一次 LLM 請求的 prompt token 數 ≈ 當前已用容量
	lastUsage        *core.Usage // 最近一次 LLM 請求的完整 usage 信息
	// usageMu 保護 lastPromptTokens/lastUsage：串流 loop 喺唔持 sessMu 時寫入，
	// 而 DebugSnapshot（另一 goroutine）要讀 lastPromptTokens，故用專鎖避免跨 goroutine
	// 競爭。
	usageMu sync.Mutex

	// 沙箱策略（宿主經 DSC_SANDBOX_POLICY 傳入，缺省 workspace-write）：渲染进
	// system prompt 的 sandbox:policy 上下文片段（对齐 DSH），让模型知道当前文件
	// 策略与工作区真实根路径，避免臆造不存在的 /workspace 虚拟路径而陷入死循环。
	sandboxPolicy string

	// 历史注入条数上限（-1 = 不限制，缺省；0 = 不注入历史；>0 = 只注入最近 N 条）。
	// 会话级设置以 log-only history/limit 事件持久化（每次 Run 折叠还原），
	// 无事件时退回 DSC_HISTORY_INJECTION 环境变量默认。用于控制本地模型预填充长度。
	historyInjection int

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

	// 重复工具调用提醒（对齐 DSH repeat-tool-reminder）：链状态进程本地。
	repeatChainName      string
	repeatChainCanonical string
	repeatChainCount     int
	// 配置：阈值（DSC_REPEAT_THRESHOLDS，缺省 3,5,8）与排除工具（DSC_REPEAT_EXCLUDE，
	// 缺省 todo_write——记录类工具穿插不掩盖循环）。
	repeatThresholds []int
	repeatExclude    []string
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

func (a *ReactLoopAgent) Run(ctx context.Context, input string, images []string) (*core.AgentResult, error) {
	return a.runLoop(ctx, input, images, nil)
}

// RunStream 以流式方式执行循环：LLM 文本增量、工具调用提示以帧的形式发送到通道，关闭表示结束
func (a *ReactLoopAgent) RunStream(ctx context.Context, input string, images []string) (<-chan *core.RunStreamResponse, error) {
	ch := make(chan *core.RunStreamResponse)
	go func() {
		defer close(ch)
		var emittedErr bool
		_, err := a.runLoop(ctx, input, images, func(item *core.RunStreamResponse) {
			if item.Status == "error" {
				emittedErr = true
			}
			ch <- item
		})
		// runLoop 在早期失败（serviceID 未设置 / 连接失败等）时不会 emit 錯誤幀，
		// 這裡補發一幀，避免錯誤被吞掉導致 TUI 靜默無響應
		if err != nil && !emittedErr {
			ch <- &core.RunStreamResponse{Status: "error", Error: err.Error()}
		}
	}()
	return ch, nil
}

// runLoop 是 Agent 的核心循环；emit 非空时输出流式帧（文本增量 / 工具提示 / 结束状态），否则保持非流式
func (a *ReactLoopAgent) runLoop(ctx context.Context, input string, images []string, emit func(*core.RunStreamResponse)) (*core.AgentResult, error) {
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

	// 事件溯源会话：首轮从磁盘恢复或新建（多会话 Store，默认会话按当前工作区
	// 项目命名——同项目跨时期共享历史、不同项目隔离，不再硬编码 default.jsonl），
	// 并构建完整 system prompt；工具列表变化时重建 system prompt。
	// 历史以事件追加进 session（对齐 DSH：模型可见即已记录），不再维护独立消息数组。
	a.sessMu.Lock()
	if a.sess == nil {
		projectKey := session.SessionKeyForProject(os.Getenv("DSC_WORKSPACE_ROOT"))
		restored, err := a.store.Ensure(projectKey)
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
	// 历史注入条数每次 Run 从事件日志折叠（/settings history 已提交的变更即时生效）；
	// 无记录时保留 newAgent 读入的 DSC_HISTORY_INJECTION 部署默认（缺省 -1 = 不限制）。
	if limit, found := session.FoldHistoryLimit(a.sess.Events()); found {
		a.historyInjection = limit
	}
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
	sess.Append(session.UserMessage, &session.UserMessageData{Content: input, Source: "user", Images: images}, &session.SurfaceOp{Op: session.SurfaceAppend})

	// cancelLoop 返回被取消的结果
	cancelResult := func() (*core.AgentResult, error) {
		return &core.AgentResult{
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
	// 包装 emit：为每帧自动填充对齐 DSH 的轮/步编号（轮=一次受理输入的排空，
	// 步=一次模型请求及其引发的工具执行），供 TUI 状态行实时显示当前进度。
	// 闭包按引用捕获 turnNo/stepNo，故每次发射都取当前步。
	if emit != nil {
		baseEmit := emit
		emit = func(f *core.RunStreamResponse) {
			f.Turn = int32(turnNo)
			f.Step = int32(stepNo)
			baseEmit(f)
		}
		// 待办投影帧（对齐 DSH FoldTodos）：turn/start 使上一轮计划失效，
		// 通知 TUI 清空待办面板；面板内容随后由 todo_write 成功结果帧驱动。
		// 无需 /todo 手动清理，也不依赖模型主动清空——新一轮即自动让位。
		emit(&core.RunStreamResponse{Status: "todo"})
	}
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

		// 请求历史由会话 surface 派生（system prompt 前置），不再依赖独立消息数组。
		// 派生与注入计数快照在同一 sessMu 临界区内完成（与 InjectMessage 互斥），
		// 保证「已发送给模型的消息」与「已消费的注入」原子一致，避免注入恰好落在
		// 两者之间被漏检。
		a.sessMu.Lock()
		msgs := sess.DeriveMessagesLimited(a.sysPrompt, a.historyInjection)
		iterPendingInjects := a.pendingInjects
		a.sessMu.Unlock()

		// 上下文自动压缩（对齐 DSH compaction-basic 的 pre-step 压力检查）：
		// - 已用容量优先取上次请求服务端报告的 prompt token（精确）；首次请求（含重启
		//   恢复历史会话）无该值，退回字符启发式估算（对齐 DSH tokenMeter 的
		//   "字符数 + 结构开销" 回退），确保重启后第一发请求不会把整段历史直接灌给模型。
		// - 超过阈值（默认 80% 窗口）时压缩最旧前缀，保留近期尾部（默认 16% 窗口）逐字，
		//   而非把全部历史折叠成单一摘要。
		// - 历史注入设置了条数上限（historyInjection >= 0）时跳过压缩：注入条数本身就是
		//   上下文的硬边界，压缩会与截断重复且其 surface 索引基于全量历史，不再适用。
		compacted := false
		promptTokens := int(a.lastPromptTokens)
		if a.contextWindow > 0 && a.historyInjection < 0 {
			// 已用容量取服务端上报值与本地字符估算的较大者：启用提示缓存的接口（如
			// llama.cpp）可能只上报未缓存的新增 token（prompt_tokens 远小于真实上下文），
			// 本地估算作为下界兜底，避免压缩判定被低估的 usage 推迟甚至永不触发。
			if est := estimatePromptTokens(msgs); est > promptTokens {
				promptTokens = est
			}
		}
		if a.contextWindow > 0 && a.historyInjection < 0 && promptTokens >= a.contextWindow*8/10 {
			if emit != nil {
				emit(&core.RunStreamResponse{
					Output: fmt.Sprintf("\n[上下文压缩: 已用 %d%% 容量，即将压缩对话历史]\n",
						promptTokens*100/a.contextWindow),
					Status: "tool",
				})
			}
			// 计算保留尾部：默认 16% 窗口（对齐 DSH retainRatio 0.16），从末尾向前累积
			// 直至预算用尽；至少 1024 token，且绝不压缩刚追加的当前用户消息（不可分尾部）。
			nodes := sess.SurfaceNodes()
			if len(nodes) >= 2 {
				retainTokens := a.contextWindow * 16 / 100
				if retainTokens < 1024 {
					retainTokens = 1024
				}
				// msgs[0] 为 system（若 sysPrompt 非空），其余与 surface 节点一一对应
				offset := 0
				if a.sysPrompt != "" {
					offset = 1
				}
				keep := len(nodes) // 首个被逐字保留的节点下标
				acc := 0
				for j := len(nodes) - 1; j >= 1; j-- {
					acc += estimateMessageTokens(msgs[offset+j])
					if acc > retainTokens {
						break
					}
					keep = j
				}
				if keep >= len(nodes) {
					keep = len(nodes) - 1 // 连最近一条都超预算：压缩到至少保留最后一条
				}
				summary, err := a.compactHistory(ctx, llmClient, msgs, availableTools)
				if err != nil {
					if emit != nil {
						emit(&core.RunStreamResponse{Status: "error", Error: err.Error()})
					}
					return nil, err
				}
				// 压缩落地（surface replace）：追加 CompactionSummary 事件遮蔽最旧前缀
				// [0, keep-1]，尾部 [keep, len-1] 保持逐字。原事件保留在日志（append-only
				// 无损，可回放/恢复）。
				sess.Append(session.CompactionSummary, &session.CompactionSummaryData{
					Content: "以下是此前对话的压缩摘要，请基于它继续当前任务：\n" + summary,
				}, &session.SurfaceOp{Op: session.SurfaceReplace, Start: nodes[0], End: nodes[keep-1]})
				// 压缩后已用容量无法精确获取，重置为 0（下一轮服务端 usage 会更新为真实值）
				a.usageMu.Lock()
				a.lastPromptTokens = 0
				a.usageMu.Unlock()
				compacted = true
			}
		}
		// 压缩后 surface 已重写，重新派生请求历史（注入计数快照一并刷新，避免已入
		// 历史的注入在收尾时被误判为新注入而重复发送）
		if compacted {
			a.sessMu.Lock()
			msgs = sess.DeriveMessagesLimited(a.sysPrompt, a.historyInjection)
			iterPendingInjects = a.pendingInjects
			a.sessMu.Unlock()
		}

		// 稳定排序工具目录（按名称），消除无关的顺序差异导致请求前缀字节变化、
		// 命中前缀缓存失效（对齐 REX 的 normalizeToolSchemas；顺序不影响功能）。
		sort.Slice(availableTools, func(i, j int) bool {
			return availableTools[i].Name < availableTools[j].Name
		})

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
				emit(&core.RunStreamResponse{Status: "error", Error: err.Error()})
				return nil, fmt.Errorf("LLM chat stream failed: %w", err)
			}
			for {
				cr, err := s.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					emit(&core.RunStreamResponse{Status: "error", Error: err.Error()})
					return nil, fmt.Errorf("LLM chat stream recv failed: %w", err)
				}
				if cr.Error != "" {
					emit(&core.RunStreamResponse{Status: "error", Error: cr.Error})
					return nil, fmt.Errorf("LLM stream error: %s", cr.Error)
				}
				// 記錄 prompt 用量（≈ 當前上下文已用容量）；該值在 finish 分片由服務端返回
				if cr.Usage != nil {
					a.usageMu.Lock()
					a.lastPromptTokens = cr.Usage.PromptTokens
					a.lastUsage = core.UsageFromProto(cr.Usage)
					a.usageMu.Unlock()
				}
				content += cr.Content
				if cr.Content != "" {
					emit(&core.RunStreamResponse{Output: cr.Content, Status: "streaming"})
				}
				if cr.Reasoning != "" {
					emit(&core.RunStreamResponse{Reasoning: cr.Reasoning, Status: "reasoning"})
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
				PromptTokens:             a.lastUsage.PromptTokens,
				CompletionTokens:         a.lastUsage.CompletionTokens,
				TotalTokens:              a.lastUsage.TotalTokens,
				CacheReadInputTokens:     a.lastUsage.CacheReadInputTokens,
				CacheCreationInputTokens: a.lastUsage.CacheCreationInputTokens,
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
						emit(&core.RunStreamResponse{
							Output: fmt.Sprintf("\n[Goal Round %d/%d]\n", a.goalRounds, g.MaxGoalRounds),
							Status: "tool",
						})
					}
					continue
				}
				// 竞态修复：模型最后一次输出期间 TUI 实时注入了新用户消息（尚未进入本轮
				// 派生的请求历史）。若就此收尾，会出现「消息已发出但工作停止」：消息已写入
				// 会话 surface，却没有任何一轮再去发送给模型。检测到快照后有新注入则继续
				// 下一轮（消息已被下轮派生，不会重复触发，也不会死循环）。
				if a.hasNewInjectedSince(iterPendingInjects) {
					if emit != nil {
						emit(&core.RunStreamResponse{
							Output: "\n[已收到新消息，继续处理]\n",
							Status: "tool",
						})
					}
					continue
				}
			}
			sess.Append(session.TurnEnd, &session.TurnData{Turn: turnNo, Reason: "completed"}, nil)
			if emit != nil {
				// success 幀攜帶當前已用容量，供 TUI 標題欄顯示「已用/總容量」
				usage := &core.Usage{
					PromptTokens: a.lastPromptTokens,
				}
				if a.lastUsage != nil {
					usage.CompletionTokens = a.lastUsage.CompletionTokens
					usage.TotalTokens = a.lastUsage.TotalTokens
					usage.CacheReadInputTokens = a.lastUsage.CacheReadInputTokens
					usage.CacheCreationInputTokens = a.lastUsage.CacheCreationInputTokens
				} else {
					usage.TotalTokens = a.lastPromptTokens
				}
				emit(&core.RunStreamResponse{
					Status: "success",
					Usage:  usage,
				})
			}
			return &core.AgentResult{
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
			// Usage 随帧携带：本步模型请求完成后即可刷新容量，不必等轮末 success 帧。
			if emit != nil {
				emit(&core.RunStreamResponse{Output: fmt.Sprintf("\n[调用工具: %s]\n", tc.Name), Status: "tool", ToolName: tc.Name, ToolArgs: tc.ArgumentsJson, Usage: a.usageSnapshot()})
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
				// owner 隔离：附带调用方会话标识（job 工具按此授权）
				a.sessMu.Lock()
				if a.sess != nil {
					toolReq.SessionId = a.sess.ID()
				}
				a.sessMu.Unlock()
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
					emit(&core.RunStreamResponse{Output: fmt.Sprintf("\n[工具结果: %s 错误] %s\n", tc.Name, toolResp.Error), Status: "tool", ToolName: tc.Name, ToolResult: toolResp.Error, Error: toolResp.Error, Usage: a.usageSnapshot()})
				}
			} else {
				sess.Append(session.ToolResult, &session.ToolResultData{
					Turn: turnNo, Step: stepNo, CallID: tc.Id,
					Content: toolResp.Content,
				}, &session.SurfaceOp{Op: session.SurfaceAppend})
				// 把工具結果輸出到流，供 TUI 渲染 REX 式结果卡片；
				// 成功结果帧附带 ToolArgs（供 TUI 更新待办面板）与 ToolView（结构化视图 spec，TUI 统一渲染）
				if emit != nil {
					emit(&core.RunStreamResponse{Output: fmt.Sprintf("\n[工具结果: %s]\n%s\n", tc.Name, toolResp.Content), Status: "tool", ToolName: tc.Name, ToolArgs: tc.ArgumentsJson, ToolResult: toolResp.Content, ToolView: toolResp.ViewJson, Usage: a.usageSnapshot()})
				}
			}
			// 重复工具调用提醒（对齐 DSH repeat-tool-reminder）：链检测在每次调用后，
			// 达阈值时在工具结果之后注入合成 user message（source guard）
			if reminder := a.repeatGuardTrack(tc.Name, canonicalArgs(tc.ArgumentsJson)); reminder != "" {
				sess.Append(session.UserMessage, &session.UserMessageData{
					Content: reminder, Source: "guard",
				}, &session.SurfaceOp{Op: session.SurfaceAppend})
			}
		}
		// 步骤结束（log-only）
		sess.Append(session.StepEnd, &session.StepData{Turn: turnNo, Step: stepNo}, nil)

		// 宿主 goal 工具标记 complete/blocked：物理轮次在本步骤后停止（对齐 DSH concludeTurn）
		if concludeTurn {
			sess.Append(session.TurnEnd, &session.TurnData{Turn: turnNo, Reason: "goal-concluded"}, nil)
			if emit != nil {
				usage := &core.Usage{PromptTokens: a.lastPromptTokens}
				if a.lastUsage != nil {
					usage.CompletionTokens = a.lastUsage.CompletionTokens
					usage.TotalTokens = a.lastUsage.TotalTokens
					usage.CacheReadInputTokens = a.lastUsage.CacheReadInputTokens
					usage.CacheCreationInputTokens = a.lastUsage.CacheCreationInputTokens
				} else {
					usage.TotalTokens = a.lastPromptTokens
				}
				emit(&core.RunStreamResponse{Status: "success", Usage: usage})
			}
			return &core.AgentResult{Output: content, Status: "success"}, nil
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
				emit(&core.RunStreamResponse{Output: "Single turn completed with tool errors", Status: "error"})
			}
			return &core.AgentResult{
				Output: "Single turn completed with tool errors",
				Status: "error",
			}, nil
		}
		if emit != nil {
			emit(&core.RunStreamResponse{Status: "success"})
		}
		return &core.AgentResult{
			Output: "Single turn completed with tools executed",
			Status: "success",
		}, nil
	}
	if emit != nil {
		// Error 字段必須帶上，否則 TUI 只按 Status 判斷、看不到任何提示，造成「莫名停止」
		emit(&core.RunStreamResponse{Output: "Max iterations reached", Status: "error", Error: "达到最大轮次上限，自动停止"})
	}
	return &core.AgentResult{
		Output: "Max iterations reached",
		Status: "error",
	}, nil
}

// usageSnapshot 生成当前步骤请求的用量快照：优先取服务端返回的 lastUsage，
// 否则退化为仅携带最近一次 prompt 用量。随工具帧携带，供 TUI 在每步工具调用
// 后实时刷新容量显示（对齐 REX 每步 usage 事件即时更新统计行的观感）。
func (a *ReactLoopAgent) usageSnapshot() *core.Usage {
	usage := &core.Usage{PromptTokens: a.lastPromptTokens}
	if a.lastUsage != nil {
		usage.CompletionTokens = a.lastUsage.CompletionTokens
		usage.TotalTokens = a.lastUsage.TotalTokens
		usage.CacheReadInputTokens = a.lastUsage.CacheReadInputTokens
		usage.CacheCreationInputTokens = a.lastUsage.CacheCreationInputTokens
	} else {
		usage.TotalTokens = a.lastPromptTokens
	}
	return usage
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

	// PTC 呈现模式（DSC_PTC=1 开启）：引导用 run_code 组合多步 + 注入 run_code
	// 可调用工具 SDK 清单；个别工具仍可照 call（fallback，不强求隐藏）。
	if s := a.ptcSDKContext(ctx, toolClient); s != "" {
		parts = append(parts, s)
	}

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

	// 沙箱策略上下文（对齐 DSH sandbox:policy 运行时上下文快照）：让模型知道当前
	// 文件策略与工作区真实根路径，避免臆造不存在的 /workspace 虚拟路径而陷入
	// 「虚拟路径访问出错、真实路径又被拦」的死循环。
	if policy := a.sandboxPolicyContext(); policy != "" {
		parts = append(parts, policy)
	}
	return strings.Join(parts, "\n\n")
}

// ptcEnabled 是否开启 PTC 呈现模式：环境变量 DSC_PTC，或处于 ptc preset（DSC_MODE=ptc）。
func ptcEnabled() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DSC_MODE")), "ptc") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DSC_PTC"))) {
	case "1", "true", "on", "ptc", "yes":
		return true
	}
	return false
}

// ptcSDKContext 返回 PTC 呈现模式下的系统提示段落：引导模型用 run_code 组合多步，
// 并把 SDK 工具清单注入。宿主在 PTC 折叠下只返回 run_code，其描述已承载「程序内
// 可调工具」（对齐 DSH presentation：直接调用折叠为唯一 run_code，其余经 SDK）。
// 非 PTC 模式返回空串（不注入，保持既有 prompt 不变）。
func (a *ReactLoopAgent) ptcSDKContext(ctx context.Context, toolClient proto.ToolServiceClient) string {
	if !ptcEnabled() {
		return ""
	}
	lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	resp, err := toolClient.ListTools(lctx, &proto.ListToolsRequest{})
	cancel()
	if err != nil {
		return ""
	}
	var sdk string
	for _, t := range resp.Tools {
		if t.Name == "run_code" {
			sdk = t.Description
			break
		}
	}
	if sdk == "" {
		sdk = formatPTCTools(resp.Tools)
	}
	return "You are in PTC (programmatic tool composition) presentation mode. For multi-step or batched " +
		"work, prefer composing ONE Lua program and running it via run_code instead of issuing many " +
		"individual tool calls; the tools callable inside the program are declared in run_code's SDK.\n\n" +
		sdk + "\n\n" + ptcLanguageSpec()
}

// ptcLanguageSpec 返回 go-lua 严格 Lua 方言的语言规范速查，便于模型快速上手写 run_code 程序。
func ptcLanguageSpec() string {
	return "Strict Lua dialect (go-lua: Lua 5.1 syntax, 5.3 integers, flow-sensitive type checker) quick reference:\n" +
		"- Tables/arrays are 1-based: items[1] is the first element (not items[0]); '#t' is sequence length; absent keys read as nil.\n" +
		"- nil != false; '--' starts a comment; 'local' scopes a variable; no trailing semicolons.\n" +
		"- Typed annotations are supported and recommended: local n:number = 1; local xs:{string} = {...}; " +
		"type P = {x:number, y:number}; local f:(number,number)->number = function(a,b) return a+b end.\n" +
		"- Nullable/optional and union types: T? ; type Event = Exit | Message. The checker narrows after a nil/type test " +
		"(e.g. 'if not x then return end' then use x.field safely; 'assert(x ~= nil)').\n" +
		"- Call a tool as its same-name function with ONE table argument (mytool{arg=...}); a failed tool resolves to nil, not an error."
}

// formatPTCTools 把 run_code 可调用工具（不含 run_code 自身）格式化为 PTC SDK 清单。
func formatPTCTools(tools []*proto.Tool) string {
	var b strings.Builder
	b.WriteString("You are in PTC (programmatic tool composition) presentation mode. For any " +
		"multi-step or batched operation, prefer composing ONE Lua script and running it via " +
		"run_code instead of issuing many individual tool calls (individual tools remain " +
		"available as fallback). Inside run_code every tool below is exposed as a same-name Lua " +
		"function (call it as <name>{args}); the script's top-level return is the result.\n" +
		"run_code SDK (tools callable inside a program):")
	for _, t := range tools {
		if t.Name == "run_code" {
			continue
		}
		desc := oneLinePrompt(t.Description)
		if len(desc) > 90 {
			desc = desc[:87] + "..."
		}
		fmt.Fprintf(&b, "\n- %s: %s", t.Name, desc)
	}
	return b.String()
}

// oneLinePrompt 把多行描述压成单行（用空行分隔，供 prompt 内紧凑展示）。
func oneLinePrompt(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// sandboxPolicyContext 渲染当前沙箱策略上下文片段。workspace-write 时携带工作区
// 真实根路径（对齐 DSH renderPolicyContext 以真实路径呈现根的约定）；/workspace
// 仍作为该根的别名被编辑器工具与 sandbox 接受，但 shell 等原生命令只能访问真实路径。
func (a *ReactLoopAgent) sandboxPolicyContext() string {
	ws := os.Getenv("DSC_WORKSPACE_ROOT")
	if ws == "" {
		ws = "."
	}
	ws = filepath.ToSlash(ws)
	// 对齐 DSH renderPolicyContext：明确「别净凭政策拒绝一次必要修改——照常尝试可用工具，
	// 并按工具返回的拒绝/升级指引行事」，避免模型因政策提示而过早放弃。
	switch strings.ToLower(strings.TrimSpace(a.sandboxPolicy)) {
	case "readonly", "read-only":
		return "Current DSC file policy: read-only. Available tool operations cannot modify files in the standing mode. " +
			"Do not refuse a required modification from this policy alone: try an available tool normally and follow any denial and escalation guidance it returns."
	case "full", "full-access":
		return "Current DSC file policy: danger-full-access. The DSC file sandbox does not restrict file modifications by available tool operations."
	default: // workspace / workspace-write（缺省，为便利而非对齐 DSH 默认的 read-only）
		return "Current DSC file policy: workspace-write. Available tool operations may modify files under the session workspace: " +
			strconv.Quote(ws) + `. The "/workspace" prefix is an alias for that root; native commands (e.g. shell) can only use the real path.`
	}
}

// compactSystemPrompt 上下文壓縮指令：要求模型只輸出精簡摘要，不添加額外解釋。
const compactSystemPrompt = "你是对话压缩器。请将下面的对话历史压缩成一段精简但信息完整的摘要，" +
	"保留用户意图、已执行的工具调用及其结果、以及所有关键的中间结论，以便在后续对话中无需原始记录也能继续。" +
	"只输出压缩后的摘要，不要输出任何解释、前言或结尾。"

// estimateTextTokens 估算一段文本的 token 数（对齐 rex guardian/estimateTokens）：
// 取「字节数/4」与「rune 数」的较大者。英文按 /4（约 4 字符 1 token）；CJK（UTF-8
// 每字 3 字节）字节/4 会低估，故回调为 rune 数（每字按 1 token），避免中文会话被
// 笼统 /4 低估约 4 倍而撑爆上下文。
func estimateTextTokens(s string) int {
	bytes := len(s)
	runes := utf8.RuneCountInString(s)
	if byBytes := (bytes + 3) / 4; byBytes > runes {
		return byBytes
	}
	return runes
}

// estimateMessageTokens 估算单条消息的 token 数（对齐 DSH tokenMeter 的
// "字符数 + 结构开销" 回退，不依赖精确 tokenizer）：文本估算 + 每条消息的固定
// 结构开销（角色、tool_call_id 等），工具调用额外计入名称与参数；图像按
// DeepSeek 文档的单图 token 上限（384）估算，避免 base64 字节数被误放大。
func estimateMessageTokens(m *proto.Message) int {
	toks := estimateTextTokens(m.Content)
	if m.Role == "tool" {
		toks += 6 // tool 角色 + tool_call_id 开销
	} else {
		toks += 4
	}
	for _, img := range m.Images {
		if img == "" {
			continue
		}
		toks += 384 // 单图 token 上限（视觉模型按尺寸换算）
	}
	for _, tc := range m.ToolCalls {
		toks += estimateTextTokens(tc.Name) + estimateTextTokens(tc.ArgumentsJson) + 8
	}
	return toks
}

// estimatePromptTokens 估算整份请求历史（含 system 前缀）的 token 数。
func estimatePromptTokens(msgs []*proto.Message) int {
	total := 0
	for _, m := range msgs {
		total += estimateMessageTokens(m)
	}
	return total
}

// compactHistory 當上下文已用容量超過 80% 時觸發：讓模型把派生歷史壓縮成摘要。
// 返回摘要文本（不含 system 消息，避免把基礎指令與技能索引壓進摘要），
// 由調用方決定落點（重建會話或 surface replace 遮蔽）。
// max_tokens 設為窗口淨餘（contextWindow - 估算輸入）的真實值，保證壓縮結果能放入
// 剩餘空間——基於字符估算而非上次請求的 usage，重啟恢復場景同樣成立。
func (a *ReactLoopAgent) compactHistory(ctx context.Context, llmClient proto.LLMServiceClient, msgs []*proto.Message, tools []*proto.Tool) (string, error) {
	if a.contextWindow <= 0 {
		return "", nil
	}
	inputTokens := estimatePromptTokens(msgs) + 64 // 压缩指令与结构开销
	remaining := a.contextWindow - inputTokens
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

// SetHistoryInjection 设置当前会话的历史注入条数上限：追加 log-only history/limit
// 事件并落盘（事件溯源：恢复/切换/fork 都能折叠还原），无需重建 system prompt。
// 同时立即更新内存值（当前 Run 后续 step 即按新值派生，无需等下一次 Run 折叠）。
// count < 0 表示不限制；0 表示不注入历史；> 0 表示只注入最近 count 条派生消息。
func (a *ReactLoopAgent) SetHistoryInjection(ctx context.Context, count int) error {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	if a.sess == nil {
		return fmt.Errorf("set history injection: session not loaded")
	}
	a.sess.Append(session.HistoryLimit, &session.HistoryLimitData{Count: count}, nil)
	if err := a.store.Save(a.sess); err != nil {
		return fmt.Errorf("set history injection: %w", err)
	}
	a.historyInjection = count // 立即生效（history off/0 后本会话后续消息不再注入历史）
	fmt.Printf("[Agent Loop] history injection limit set to %d\n", count)
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
func (a *ReactLoopAgent) Version(ctx context.Context) string { return "1.1.0" } // 消息支持图像附件

// InjectMessage 将一条用户消息实时注入到当前运行中会话的历史末端。
// 运行中的 runLoop 每步都从会话 surface 重新派生请求历史（DeriveMessages），
// 因此这里追加的 UserMessage 表面事件会在下一次 LLM 迭代即被模型看到——
// 无需停止或等待本轮完成（对齐 TUI 正在工作中的实时输入）。
func (a *ReactLoopAgent) InjectMessage(ctx context.Context, content string, images []string) error {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	if a.sess == nil {
		return fmt.Errorf("inject message: session not loaded")
	}
	a.sess.Append(session.UserMessage,
		&session.UserMessageData{Content: content, Source: "user", Images: images},
		&session.SurfaceOp{Op: session.SurfaceAppend})
	a.pendingInjects++ // 记录一次尚未被 runLoop 消费的注入（收尾前据此检测是否继续）
	fmt.Printf("[Agent Loop] injected user message (%d chars) into running session\n", len(content))
	return nil
}

// hasNewInjectedSince 返回是否自注入基线（iterInjects，本迭代派生请求历史时快照的
// pendingInjects）之后又有新的 InjectMessage 注入。runLoop 在模型无工具调用收尾前调用：
// 命中说明模型输出期间 TUI 注入了新用户消息，本轮不得结束，须继续下一轮处理。
func (a *ReactLoopAgent) hasNewInjectedSince(iterInjects int) bool {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	return a.pendingInjects > iterInjects
}

// DebugSnapshot 返回 agent 当前运行时的调试快照，供 ADMIN API 的 DEBUGGER 端点
// 与自动化测试观察代理内部状态：会话历史（含实时注入的消息）、token 用量、
// turn 计数与 plan/goal 状态。
func (a *ReactLoopAgent) DebugSnapshot(ctx context.Context) (*core.AgentDebugSnapshot, error) {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	if a.sess == nil {
		return nil, fmt.Errorf("debug snapshot: session not loaded")
	}

	// lastPromptTokens 由串流 loop 在 usageMu 下寫入（不在 sessMu 下），故分開取鎖讀
	a.usageMu.Lock()
	lastPromptTokens := a.lastPromptTokens
	a.usageMu.Unlock()

	snap := &core.AgentDebugSnapshot{
		SessionID:        a.sess.ID(),
		TurnCount:        a.turnCounter,
		PlanActive:       a.planActive,
		LastPromptTokens: lastPromptTokens,
	}

	// goal 状态由事件日志折叠
	if g := session.FoldGoal(a.sess.Events()); g != nil {
		snap.Goal = &core.AgentGoalDebugInfo{
			Phase: g.Phase, Revision: g.Revision,
			MaxRounds: g.MaxGoalRounds, Objective: g.Objective,
		}
	}

	// 派生的请求历史（与下次 LLM 请求一致，含实时注入的消息与历史注入条数限制）
	for _, m := range a.sess.DeriveMessagesLimited(a.sysPrompt, a.historyInjection) {
		snap.Messages = append(snap.Messages, &core.AgentDebugMessage{Role: m.Role, Content: m.Content})
	}
	return snap, nil
}

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

// newAgent 从宿主注入的环境变量构建 ReactLoopAgent（配置读取 + 会话存储初始化）。
// broker 由 SDK 在 gRPC server 建立时经 AgentBroker 回调注入（仅该阶段可用）。
func newAgent() (*ReactLoopAgent, error) {
	agent := &ReactLoopAgent{}
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
	// 讀取宿主傳入的沙箱策略（DSC_SANDBOX_POLICY，缺省 workspace-write）
	agent.sandboxPolicy = os.Getenv("DSC_SANDBOX_POLICY")
	if agent.sandboxPolicy == "" {
		agent.sandboxPolicy = "workspace-write"
	}
	// 讀取宿主傳入的历史注入条数默认值（DSC_HISTORY_INJECTION）：-1 不限制（缺省），
	// 0 不注入历史，>0 只注入最近 N 条。会话内可通过 /settings history 以 history/limit
	// 事件覆盖（事件折叠优先于此默认值）。
	agent.historyInjection = -1
	if v := os.Getenv("DSC_HISTORY_INJECTION"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= -1 {
			agent.historyInjection = n
		}
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
	// 重复工具提醒阈值（DSC_REPEAT_THRESHOLDS，逗号分隔升序，缺省 3,5,8）
	agent.repeatThresholds = []int{3, 5, 8}
	if v := os.Getenv("DSC_REPEAT_THRESHOLDS"); v != "" {
		var ts []int
		for _, part := range strings.Split(v, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n >= 2 {
				ts = append(ts, n)
			}
		}
		if len(ts) > 0 {
			sort.Ints(ts)
			agent.repeatThresholds = ts
		}
	}
	// 重复提醒排除工具（DSC_REPEAT_EXCLUDE，逗号分隔，缺省 todo_write）
	agent.repeatExclude = []string{"todo_write"}
	if v := os.Getenv("DSC_REPEAT_EXCLUDE"); v != "" {
		agent.repeatExclude = strings.Split(v, ",")
	}
	// 多会话事件日志存储（DSC_SESSION_DIR，缺省落在插件工作目录下的 sessions/）
	dir := os.Getenv("DSC_SESSION_DIR")
	if dir == "" {
		dir = "sessions"
	}
	store, err := session.NewStore(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to open session store: %w", err)
	}
	agent.store = store
	return agent, nil
}

// main 以公共 SDK（dsc-sdk）声明式启动：SDK 复用宿主 core.AgentGRPCPlugin 提供
// AgentService + 元数据，并经 AgentBroker 回调在 gRPC server 建立时注入宿主 broker
// （重写自旧的 customAgentPlugin/agentGRPCServer 样板）。
func main() {
	agent, err := newAgent()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-react-loop: %v\n", err)
		os.Exit(2)
	}

	sdk := dsc.New(dsc.Config{Name: "agent-react-loop", Version: "1.1.0", Type: dsc.TypeAgent})
	sdk.Agent(agent)
	sdk.AgentBroker(func(b *dsc.AgentBroker) error {
		agent.broker = b
		return nil
	})
	sdk.Serve()
}
