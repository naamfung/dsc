package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"dsc/plugin"
	"dsc/proto"
	"google.golang.org/grpc"
)

// LLMProxyServer 实现 LLMService，转发到 LLMProvider
type LLMProxyServer struct {
	proto.UnimplementedLLMServiceServer
	mgr     *plugin.Manager
	llmName string
}

func (s *LLMProxyServer) getLLM() plugin.LLMProvider {
	llm, ok := s.mgr.GetLLM(s.llmName)
	if !ok {
		return nil
	}
	return llm
}

func (s *LLMProxyServer) Chat(ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	llm := s.getLLM()
	if llm == nil {
		return nil, fmt.Errorf("LLM not available")
	}
	messages := make([]plugin.Message, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = plugin.Message{Role: m.Role, Content: m.Content}
	}
	tools := make([]plugin.Tool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = plugin.Tool{Name: t.Name, Description: t.Description, ParametersJSON: t.ParametersJson}
	}
	resp, err := llm.Chat(ctx, messages, tools)
	if err != nil {
		return nil, err
	}
	toolCalls := make([]*proto.ToolCall, len(resp.ToolCalls))
	for i, tc := range resp.ToolCalls {
		argsJSON, _ := json.Marshal(tc.Arguments)
		toolCalls[i] = &proto.ToolCall{Name: tc.Name, ArgumentsJson: string(argsJSON)}
	}
	return &proto.ChatResponse{
		Content:      resp.Content,
		FinishReason: resp.FinishReason,
		ToolCalls:    toolCalls,
	}, nil
}

func (s *LLMProxyServer) Name(ctx context.Context, req *proto.NameRequest) (*proto.NameResponse, error) {
	llm := s.getLLM()
	if llm == nil {
		return nil, fmt.Errorf("LLM not available")
	}
	return &proto.NameResponse{Name: llm.Name(ctx)}, nil
}
func (s *LLMProxyServer) Version(ctx context.Context, req *proto.VersionRequest) (*proto.VersionResponse, error) {
	llm := s.getLLM()
	if llm == nil {
		return nil, fmt.Errorf("LLM not available")
	}
	return &proto.VersionResponse{Version: llm.Version(ctx)}, nil
}
func (s *LLMProxyServer) HealthCheck(ctx context.Context, req *proto.HealthCheckRequest) (*proto.HealthCheckResponse, error) {
	llm := s.getLLM()
	if llm == nil {
		return nil, fmt.Errorf("LLM not available")
	}
	err := llm.HealthCheck(ctx)
	status := "okay"
	msg := ""
	if err != nil {
		status = "error"
		msg = err.Error()
	}
	return &proto.HealthCheckResponse{Status: status, Message: msg}, nil
}

func main() {
	mgr := plugin.NewManager(&plugin.ManagerConfig{
		PluginDir: "./plugins/",
		Handshake: plugin.Handshake,
	})
	defer mgr.Shutdown()

	llmName := os.Getenv("LLM_PROVIDER")
	if llmName == "" {
		llmName = "openai"
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	llmBinary := map[string]string{
		"openai":    "./plugins/llm-openai/llm-openai" + ext,
		"anthropic": "./plugins/llm-anthropic/llm-anthropic" + ext,
		"ollama":    "./plugins/llm-ollama/llm-ollama" + ext,
	}[llmName]
	if llmBinary == "" {
		log.Fatalf("Unknown LLM provider: %s", llmName)
	}

	if err := mgr.LoadLLM(llmName, llmBinary); err != nil {
		log.Fatalf("Failed to load LLM: %v", err)
	}
	fmt.Printf("[Main] Loaded LLM: %s\n", llmName)

	// 使用 Manager 加載 Agent
	agentBinary := "./plugins/react_loop/react_loop" + ext
	broker, serviceID, err := mgr.LoadAgentAndGetBroker("react_agent", agentBinary)
	if err != nil {
		log.Fatalf("Failed to load Agent: %v", err)
	}
	fmt.Printf("[Main] Generated serviceID: %d\n", serviceID)

	// 注册 LLM 服务（在 goroutine 中，避免阻塞）
	go func() {
		broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
			s := grpc.NewServer(opts...)
			proto.RegisterLLMServiceServer(s, &LLMProxyServer{mgr: mgr, llmName: llmName})
			return s
		})
	}()

	agent, ok := mgr.GetAgent("react_agent")
	if !ok {
		log.Fatalf("Agent not found after loading")
	}

	ctx := context.Background()
	result, err := agent.Run(ctx, "What is the weather in Tokyo?")
	if err != nil {
		log.Fatalf("Agent run failed: %v", err)
	}
	fmt.Printf("Agent Result: %+v\n", result)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\n[Main] Shutting down...")
}
