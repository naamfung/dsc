package main

import (
	"context"
	"fmt"
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

	// 新增字段
	cancelFunc    context.CancelFunc // 用於取消當前 Run
	runWg         sync.WaitGroup     // 等待 Run 完成
	shutdownMu    sync.Mutex         // 保護關閉狀態
	isShutdown    bool
}

func (a *ReactLoopAgent) SetLLMServiceID(ctx context.Context, id uint32) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.llmServiceID = id
	return nil
}

func (a *ReactLoopAgent) SetToolServiceID(ctx context.Context, id uint32) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.toolServiceID = id
	return nil
}

func (a *ReactLoopAgent) Run(ctx context.Context, input string) (*plugin.AgentResult, error) {
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

	// 连接 LLM 服务
	llmConn, err := a.broker.Dial(llmID)
	if err != nil {
		return nil, fmt.Errorf("failed to dial LLM service: %w", err)
	}
	defer llmConn.Close()
	llmClient := proto.NewLLMServiceClient(llmConn)

	// 连接 ToolService
	toolConn, err := a.broker.Dial(toolID)
	if err != nil {
		return nil, fmt.Errorf("failed to dial tool service: %w", err)
	}
	defer toolConn.Close()
	toolClient := proto.NewToolServiceClient(toolConn)

	// 在循环外获取工具列表（一次即可）
	listToolsResp, err := toolClient.ListTools(ctx, &proto.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}
	availableTools := listToolsResp.Tools

	messages := []*proto.Message{
		{Role: "system", Content: "You are a helpful assistant with access to tools."},
		{Role: "user", Content: input},
	}

	maxIterations := 5
	for i := 0; i < maxIterations; i++ {
		// 检查 ctx.Done()
		select {
		case <-ctx.Done():
			return &plugin.AgentResult{
				Output: "Agent canceled",
				Status: "error",
			}, ctx.Err()
		default:
		}

		// 上下文管理與截斷
		const maxMessages = 20
		if len(messages) > maxMessages {
			// 保留 system 和 user 第一条，删除中间，保留最后几条
			// 简单实现：保留前 2 条（system + initial user）和最后 (maxMessages-2) 条
			kept := messages[:2]
			tailStart := len(messages) - (maxMessages - 2)
			if tailStart < 2 {
				tailStart = 2
			}
			messages = append(kept, messages[tailStart:]...)
		}

		// 调用 LLM
		req := &proto.ChatRequest{
			Messages: messages,
			Tools:    availableTools,
		}
		resp, err := llmClient.Chat(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("LLM chat failed: %w", err)
		}

		// 追加助手消息（包含文本回复）
		messages = append(messages, &proto.Message{
			Role:    "assistant",
			Content: resp.Content,
		})

		// 没有工具调用 → 返回最终结果
		if len(resp.ToolCalls) == 0 {
			return &plugin.AgentResult{
				Output: resp.Content,
				Status: "success",
			}, nil
		}

		// 执行每个工具并追加结果
		for _, tc := range resp.ToolCalls {
			// 检查 ctx.Done()
			select {
			case <-ctx.Done():
				return &plugin.AgentResult{
					Output: "Agent canceled",
					Status: "error",
				}, ctx.Err()
			default:
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
				messages = append(messages, &proto.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("Error executing tool %s: %v", tc.Name, err),
					ToolCallId: tc.Id,
				})
				continue
			}
			if toolResp.Error != "" {
				messages = append(messages, &proto.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("Tool error: %s", toolResp.Error),
					ToolCallId: tc.Id,
				})
			} else {
				messages = append(messages, &proto.Message{
					Role:       "tool",
					Content:    toolResp.Content,
					ToolCallId: tc.Id,
				})
			}
		}
	}
	// 超过最大迭代次数
	return &plugin.AgentResult{
		Output: "Max iterations reached",
		Status: "error",
	}, nil
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

func main() {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"agent": &customAgentPlugin{},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}