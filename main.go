package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"dsc/plugin"
	"dsc/proto"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"gopkg.in/yaml.v3"
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

func loadConfig(path string) (*plugin.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg plugin.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// waitForService 等待 broker 服務就緒
func waitForService(broker *goplugin.GRPCBroker, id uint32) error {
	for i := 0; i < 10; i++ {
		conn, err := broker.Dial(id)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("service %d not ready", id)
}

func main() {
	// 加載配置文件
	cfgPath := os.Getenv("DSC_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		// 配置文件不存在或加載失敗時，使用默認配置
		logger := hclog.New(&hclog.LoggerOptions{
			Name:   "dsc-host",
			Level:  hclog.Info,
			Output: os.Stderr,
		})
		logger.Info("config file not found or invalid, using default config", "path", cfgPath)
	} else {
		// 設置配置中的 workspace root
		if cfg.WorkspaceRoot != "" {
			plugin.WorkspaceRoot = cfg.WorkspaceRoot
		}
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "dsc-host",
		Level:  hclog.Info,
		Output: os.Stderr,
	})

	mgr := plugin.NewManager(&plugin.ManagerConfig{
		PluginDir: "./plugins/",
		Handshake: plugin.Handshake,
		Logger:    logger,
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
		logger.Error("unknown LLM provider", "name", llmName)
		os.Exit(1)
	}

	if err := mgr.LoadLLM(llmName, llmBinary); err != nil {
		logger.Error("failed to load LLM", "name", llmName, "error", err)
		os.Exit(1)
	}
	logger.Info("llm loaded", "name", llmName)

	// 使用 Manager 加載 Agent
	agentBinary := "./plugins/react_loop/react_loop" + ext
	broker, llmServiceID, err := mgr.LoadAgentAndGetBroker("react_agent", agentBinary)
	if err != nil {
		logger.Error("failed to load Agent", "error", err)
		os.Exit(1)
	}
	logger.Info("llm service id generated", "serviceID", llmServiceID)

	// 注册 LLM 服务（在 goroutine 中，避免阻塞）
	go func() {
		broker.AcceptAndServe(llmServiceID, func(opts []grpc.ServerOption) *grpc.Server {
			s := grpc.NewServer(opts...)
			proto.RegisterLLMServiceServer(s, &LLMProxyServer{mgr: mgr, llmName: llmName})
			return s
		})
	}()

	// 注册 ToolService（在 goroutine 中，避免阻塞）
	toolServiceID := broker.NextId()
	go func() {
		broker.AcceptAndServe(toolServiceID, func(opts []grpc.ServerOption) *grpc.Server {
			s := grpc.NewServer(opts...)
			proto.RegisterToolServiceServer(s, plugin.NewToolGRPCServer(mgr))
			return s
		})
	}()
	logger.Info("tool service id generated", "serviceID", toolServiceID)

	// 等待服務就緒
	if err := waitForService(broker, llmServiceID); err != nil {
		logger.Error("failed to wait for LLM service", "error", err)
		os.Exit(1)
	}
	if err := waitForService(broker, toolServiceID); err != nil {
		logger.Error("failed to wait for tool service", "error", err)
		os.Exit(1)
	}
	logger.Info("services are ready")

	agent, ok := mgr.GetAgent("react_agent")
	if !ok {
		logger.Error("agent not found after loading")
		os.Exit(1)
	}

	ctx := context.Background()
	// 设置 Tool Service ID
	if err := agent.SetToolServiceID(ctx, toolServiceID); err != nil {
		logger.Error("failed to set tool service ID on agent", "error", err)
		os.Exit(1)
	}

	result, err := agent.Run(ctx, "What is the weather in Tokyo?")
	if err != nil {
		logger.Error("agent run failed", "error", err)
		os.Exit(1)
	}
	logger.Info("agent result", "result", result)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Info("shutting down...")
}
