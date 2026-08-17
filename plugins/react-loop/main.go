package main

import (
	"context"
	"fmt"
	"io"
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
		a.runLoop(ctx, input, func(item *plugin.RunStreamResponse) {
			ch <- item
		})
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

	// 讀取/追加當前用戶輸入到歷史
	a.historyMu.Lock()
	if len(a.history) == 0 {
		// 第一次對話，添加 system prompt
		a.history = []*proto.Message{
			{Role: "system", Content: "You are a helpful assistant with access to tools."},
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

	maxIterations := 5
	for i := 0; i < maxIterations; i++ {
		// 检查 ctx.Done()
		select {
		case <-ctx.Done():
			return cancelResult()
		default:
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
				content += cr.Content
				if cr.Content != "" {
					emit(&plugin.RunStreamResponse{Output: cr.Content, Status: "streaming"})
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
				emit(&plugin.RunStreamResponse{Status: "success"})
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

			// 向客户端提示正在调用工具
			if emit != nil {
				emit(&plugin.RunStreamResponse{Output: fmt.Sprintf("\n[调用工具: %s]\n", tc.Name), Status: "tool"})
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
			} else {
				currentHistory = append(currentHistory, &proto.Message{
					Role:       "tool",
					Content:    toolResp.Content,
					ToolCallId: tc.Id,
				})
			}
		}
	}
	// 超过最大迭代次数（持久化歷史）
	a.saveHistory(currentHistory)
	if emit != nil {
		emit(&plugin.RunStreamResponse{Output: "Max iterations reached", Status: "error"})
	}
	return &plugin.AgentResult{
		Output: "Max iterations reached",
		Status: "error",
	}, nil
}

// saveHistory 將當前的會話上下文寫回 agent 歷史，供下一輪 Run 使用
func (a *ReactLoopAgent) saveHistory(messages []*proto.Message) {
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	a.history = messages
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
			Output: item.Output,
			Status: item.Status,
			Error:  item.Error,
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