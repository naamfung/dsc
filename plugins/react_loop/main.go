package main

import (
	"context"
	"fmt"
	"sync"

	"dsc/plugin"
	"dsc/proto"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type ReactLoopAgent struct {
	broker      *goplugin.GRPCBroker
	serviceID   uint32
	mu          sync.Mutex // 保護 serviceID
}

func (a *ReactLoopAgent) SetLLMServiceID(ctx context.Context, id uint32) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.serviceID = id
	return nil
}

func (a *ReactLoopAgent) Run(ctx context.Context, input string) (*plugin.AgentResult, error) {
	fmt.Printf("[Agent Loop] Starting turn with input: %s\n", input)

	a.mu.Lock()
	id := a.serviceID
	a.mu.Unlock()
	if id == 0 {
		return nil, fmt.Errorf("serviceID not set, call SetLLMServiceID first")
	}

	// 每次调用时 Dial 连接 LLM 服务
	llmConn, err := a.broker.Dial(id)
	if err != nil {
		return nil, fmt.Errorf("failed to dial LLM service: %w", err)
	}
	defer llmConn.Close()
	llmClient := proto.NewLLMServiceClient(llmConn)

	messages := []*proto.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: input},
	}
	req := &proto.ChatRequest{
		Messages: messages,
		Tools:    nil,
	}
	resp, err := llmClient.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM chat failed: %w", err)
	}
	return &plugin.AgentResult{
		Output: resp.Content,
		Status: "success",
	}, nil
}

func (a *ReactLoopAgent) Name(ctx context.Context) string { return "react_agent" }
func (a *ReactLoopAgent) Version(ctx context.Context) string { return "1.0.0" }

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

func main() {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"agent": &customAgentPlugin{},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}