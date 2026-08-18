package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

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
	messages, tools := convertChatRequest(req)
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

// ChatStream 转发 LLM 插件的流式响应（旧版直连路径使用）
func (s *LLMProxyServer) ChatStream(req *proto.ChatRequest, stream proto.LLMService_ChatStreamServer) error {
	llm := s.getLLM()
	if llm == nil {
		return fmt.Errorf("LLM not available")
	}
	messages, tools := convertChatRequest(req)
	ch, err := llm.ChatStream(stream.Context(), messages, tools)
	if err != nil {
		return err
	}
	for item := range ch {
		if item.Error != "" {
			return fmt.Errorf("LLM stream error: %s", item.Error)
		}
		toolCalls := make([]*proto.ToolCall, len(item.ToolCalls))
		for i, tc := range item.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Arguments)
			toolCalls[i] = &proto.ToolCall{Name: tc.Name, ArgumentsJson: string(argsJSON)}
		}
		if err := stream.Send(&proto.ChatStreamResponse{
			Content:      item.Content,
			FinishReason: item.FinishReason,
			ToolCalls:    toolCalls,
		}); err != nil {
			return err
		}
	}
	return nil
}

// convertChatRequest 将 proto 请求转换为内部消息与工具列表（旧版直连路径使用）
func convertChatRequest(req *proto.ChatRequest) ([]plugin.Message, []plugin.Tool) {
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
	return messages, tools
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
	// 解析啟動參數
	logToScreen := false
	logToFile := ""
	mode := "standard" // 默認標準模式

	for i, arg := range os.Args {
		if arg == "-log" {
			if i+1 < len(os.Args) {
				nextArg := os.Args[i+1]
				if strings.HasPrefix(nextArg, "-") {
					logToScreen = true
				} else {
					logToFile = nextArg
				}
			} else {
				logToScreen = true
			}
		} else if arg == "-mode" {
			if i+1 < len(os.Args) {
				nextArg := os.Args[i+1]
				if nextArg == "minimal" || nextArg == "standard" {
					mode = nextArg
				} else {
					mode = "standard" // 默認 standard
				}
			}
		}
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "dsc-host",
		Level:  hclog.Info,
		Output: os.Stderr,
	})

	// 初始化 logger 和 pluginLogger (根據 logToFile 和 logToScreen 調整)
	var pluginLogger hclog.Logger
	var logOutput io.Writer

	if logToFile != "" {
		// 日志静默写到设定的path去
		f, err := os.OpenFile(logToFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			// 如果打開文件失敗，回退到屏幕
			logOutput = os.Stderr
			logger = hclog.New(&hclog.LoggerOptions{
				Name:   "dsc-host",
				Level:  hclog.Info,
				Output: logOutput,
			})
			pluginLogger = hclog.New(&hclog.LoggerOptions{
				Name:   "plugin",
				Level:  hclog.Info,
				Output: logOutput,
			})
		} else {
			logOutput = f
			logger = hclog.New(&hclog.LoggerOptions{
				Name:   "dsc-host",
				Level:  hclog.Info,
				Output: logOutput,
			})
			pluginLogger = hclog.New(&hclog.LoggerOptions{
				Name:   "plugin",
				Level:  hclog.Info,
				Output: logOutput,
			})
			// 確保在退出時關閉文件
			defer f.Close()
		}
	} else if logToScreen {
		// -log 无指定路径时似如今一样打印日志到屏幕
		logOutput = os.Stderr
		logger = hclog.New(&hclog.LoggerOptions{
			Name:   "dsc-host",
			Level:  hclog.Info,
			Output: logOutput,
		})
		pluginLogger = hclog.New(&hclog.LoggerOptions{
			Name:   "plugin",
			Level:  hclog.Info,
			Output: logOutput,
		})
	} else {
		// 無參數時，日誌静默放棄，不作記錄
		logger = hclog.New(&hclog.LoggerOptions{
			Name:   "dsc-host",
			Level:  hclog.NoLevel,
			Output: io.Discard,
		})
		pluginLogger = hclog.New(&hclog.LoggerOptions{
			Name:   "plugin",
			Level:  hclog.NoLevel,
			Output: io.Discard,
		})
	}

	logger.Info("starting dsc", "mode", mode)

	// 加載配置文件
	cfgPath := os.Getenv("DSC_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		// 配置文件不存在或加載失敗時，使用默認配置
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

	// 根據模式過濾插件
	filteredPlugins := []plugin.PluginEntry{}
	if mode == "minimal" {
		// 極簡模式：僅加載核心工具 (tool-str-replace-editor, tool-filesystem) 和相關依賴 (fs-observation-policy), LLM 和 Agent
		for _, p := range cfg.Plugins {
			if p.Type == "llm" || p.Type == "agent" || p.Name == "tool-str-replace-editor" || p.Name == "tool-filesystem" || p.Name == "fs-observation-policy" {
				filteredPlugins = append(filteredPlugins, p)
			}
		}
	} else {
		// 標準模式：加載所有插件
		filteredPlugins = cfg.Plugins
	}

	// 創建過濾後的 config
	filteredCfg := &plugin.Config{
		WorkspaceRoot: cfg.WorkspaceRoot,
		DefaultLLM:    cfg.DefaultLLM,
		Plugins:       filteredPlugins,
	}

	mgr := plugin.NewManager(&plugin.ManagerConfig{
		PluginDir:    "./plugins/",
		Handshake:    plugin.Handshake,
		Logger:       logger,
		PluginLogger: pluginLogger,
	})
	defer mgr.Shutdown()

	// 如果配置中有插件列表（過濾後），則從配置加載
	if len(filteredCfg.Plugins) > 0 {
		if err := mgr.LoadFromConfig(filteredCfg); err != nil {
			logger.Error("failed to load plugins from config", "error", err)
			os.Exit(1)
		}
		logger.Info("plugins loaded from config", "count", len(filteredCfg.Plugins), "mode", mode)

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
		}
		logger.Info("tui exited")

		// 完成完整的清理過程再退出
		logger.Info("shutting down...")
		mgr.Shutdown()
		os.Exit(0)
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
		}
		logger.Info("tui exited")

		// 完成完整的清理過程再退出
		logger.Info("shutting down...")
		mgr.Shutdown()
		os.Exit(0)
	}
}
