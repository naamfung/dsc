package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"dsc/plugin"
	"dsc/proto"
	"dsc/tui"
	"github.com/hashicorp/go-hclog"
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
		msg := plugin.Message{Role: m.Role, Content: m.Content}
		if len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]plugin.ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.ArgumentsJson), &args); err != nil {
					args = map[string]interface{}{}
				}
				msg.ToolCalls[j] = plugin.ToolCall{ID: tc.Id, Name: tc.Name, Arguments: args}
			}
		}
		messages[i] = msg
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
		// 使用默認配置
		cfg = &plugin.Config{
			WorkspaceRoot: "",
			DefaultLLM:    "openai",
		}
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

	// 如果配置中有插件列表，則從配置加載
	if len(cfg.Plugins) > 0 {
		if err := mgr.LoadFromConfig(cfg); err != nil {
			logger.Error("failed to load plugins from config", "error", err)
			os.Exit(1)
		}
		logger.Info("plugins loaded from config", "count", len(cfg.Plugins))

		// 啟動管理 API
		adminAddr := os.Getenv("DSC_ADMIN_ADDR")
		if adminAddr == "" {
			adminAddr = ":9999"
		}
		mgr.StartAdmin(adminAddr)
		logger.Info("admin api started", "addr", adminAddr)

		// 获取 Agent 并运行 TUI 聊天界面
		agentName := mgr.GetMainAgentName()
		agent, ok := mgr.GetAgent(agentName)
		if !ok {
			logger.Error("agent not found after loading", "agentName", agentName)
			os.Exit(1)
		}

		// 从配置中提取 LLM 模型名称
		llmModelName := "Unknown"
		for _, entry := range cfg.Plugins {
			if entry.Type == "llm" && entry.Enabled {
				if v, ok := entry.Env["ANTHROPIC_MODEL"]; ok {
					llmModelName = v
				} else if v, ok := entry.Env["OPENAI_MODEL"]; ok {
					llmModelName = v
				} else if v, ok := entry.Env["OLLAMA_MODEL"]; ok {
					llmModelName = v
				}
				break
			}
		}

		ctx := context.Background()
		if err := tui.Run(agent, ctx, llmModelName); err != nil {
			logger.Error("tui run failed", "error", err)
			os.Exit(1)
		}
		logger.Info("tui exited")
	} else {
		// 兼容舊的加載方式
		llmName := os.Getenv("LLM_PROVIDER")
		if llmName == "" && cfg.DefaultLLM != "" {
			llmName = cfg.DefaultLLM
		}
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
		agentBinary := "./plugins/react-loop/react-loop" + ext
		broker, llmServiceID, err := mgr.LoadAgentAndGetBroker("react-agent", agentBinary)
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

		agent, ok := mgr.GetAgent("react-agent")
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

		// 舊路徑沒有明確的模型名稱配置，設置為空字符串，TUI會回退到agent.Name()
		llmModelNameLegacy := ""

		// 启动 TUI 聊天界面（替换原来的硬编码 Agent.Run 调用）
		if err := tui.Run(agent, ctx, llmModelNameLegacy); err != nil {
			logger.Error("tui run failed", "error", err)
			os.Exit(1)
		}
		logger.Info("tui exited")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Info("shutting down...")
}
