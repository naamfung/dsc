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
	// 捕獲 panic 並輸出到 stderr，以便在日誌靜默時能調試啟動失敗原因
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "panic recovered: %v\n", r)
		}
	}()

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

	// 加載 preset 配置文件
	presetPath := fmt.Sprintf("config/presets/%s.yaml", mode)
	presetCfg, err := loadConfig(presetPath)
	if err != nil {
		// 如果 preset 配置文件不存在或加載失敗，回退到默認 config.yaml
		logger.Info("preset config not found or invalid, using default config", "presetPath", presetPath, "error", err)
		presetCfg = nil
	}

	mgr := plugin.NewManager(&plugin.ManagerConfig{
		PluginDir:    "./plugins/",
		Handshake:    plugin.Handshake,
		Logger:       logger,
		PluginLogger: pluginLogger,
	})
	defer mgr.Shutdown()

	// fail 記錄錯誤後先清理已加載的插件子進程再退出，避免 os.Exit 跳過 defer 導致殘留孤兒進程
	fail := func(format string, args ...interface{}) {
		logger.Error(fmt.Sprintf(format, args...))
		mgr.Shutdown()
		os.Exit(1)
	}

	// 1. 加載 LLM 插件（維持原來的啟動邏輯）
	llmName := os.Getenv("LLM_PROVIDER")
	if llmName == "" && presetCfg != nil && presetCfg.DefaultLLM != "" {
		llmName = presetCfg.DefaultLLM
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
		fail("unknown LLM provider %q", llmName)
	}

	// 從主配置 config.yaml 讀取 LLM 插件的環境變量（BASE_URL / API_KEY / MODEL 等），
	// 否則 llm-openai 等插件會使用內置默認值（如 DeepSeek 地址），導致請求發往錯誤服務
	llmEnv := map[string]string{}
	if mainCfg, err := loadConfig("config.yaml"); err == nil {
		for _, entry := range mainCfg.Plugins {
			if entry.Type == "llm" && entry.Name == llmName && entry.Enabled {
				llmEnv = entry.Env
				break
			}
		}
	}

	if err := mgr.LoadLLM(llmName, llmBinary, llmEnv); err != nil {
		fail("failed to load LLM %s: %v", llmName, err)
	}
	logger.Info("llm loaded", "name", llmName)

	// 2. 加載 Agent 插件
	agentBinary := "./plugins/react-loop/react-loop" + ext
	broker, llmServiceID, err := mgr.LoadAgentAndGetBroker("react-agent", agentBinary)
	if err != nil {
		fail("failed to load Agent: %v", err)
	}
	logger.Info("llm service id generated", "serviceID", llmServiceID)

	// 3. 註冊 LLM 服務（在 goroutine 中，避免阻塞）
	go func() {
		broker.AcceptAndServe(llmServiceID, func(opts []grpc.ServerOption) *grpc.Server {
			s := grpc.NewServer(opts...)
			proto.RegisterLLMServiceServer(s, &LLMProxyServer{mgr: mgr, llmName: llmName})
			return s
		})
	}()

	// 4. 如果 preset 配置中有插件列表（tools 和 policies），則從配置加載
	if presetCfg != nil && len(presetCfg.Plugins) > 0 {
		if err := mgr.LoadToolsAndPoliciesFromConfig(presetCfg); err != nil {
			fail("failed to load plugins from preset config: %v", err)
		}
		logger.Info("plugins loaded from preset config", "count", len(presetCfg.Plugins), "mode", mode)
	}

	// 5. 注册 ToolService（在 goroutine 中，避免阻塞）
	toolServiceID := broker.NextId()
	go func() {
		broker.AcceptAndServe(toolServiceID, func(opts []grpc.ServerOption) *grpc.Server {
			s := grpc.NewServer(opts...)
			proto.RegisterToolServiceServer(s, plugin.NewToolGRPCServer(mgr))
			return s
		})
	}()
	logger.Info("tool service id generated", "serviceID", toolServiceID)

	// 6. 啟動管理 API
	adminAddr := os.Getenv("DSC_ADMIN_ADDR")
	if adminAddr == "" {
		adminAddr = ":9999"
	}
	mgr.StartAdmin(adminAddr)
	logger.Info("admin api started", "addr", adminAddr)

	// 7. 获取 Agent 并运行 TUI 聊天界面
	agentName := mgr.GetMainAgentName()
	if agentName == "" {
		agentName = "react-agent"
	}
	agent, ok := mgr.GetAgent(agentName)
	if !ok {
		fail("agent %s not found after loading", agentName)
	}
	// 告訴 Agent 主進程 ToolService 代理的 serviceID；否則 react-loop 的 toolServiceID 為 0，
	// 首條消息會直接返回 "service IDs not set" 錯誤（並被靜默吞掉，表現為無響應）
	if err := agent.SetToolServiceID(context.Background(), toolServiceID); err != nil {
		fail("failed to set tool service ID on agent: %v", err)
	}

	// 从配置中提取 LLM 模型名称
	llmModelName := "Unknown"
	// 嘗試從 presetCfg 獲取（雖然 presetCfg 中沒有 llm 配置，但保留邏輯）
	if presetCfg != nil {
		for _, entry := range presetCfg.Plugins {
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
	}
	if llmModelName == "Unknown" {
		// 從環境變量獲取
		if v := os.Getenv("OPENAI_MODEL"); v != "" {
			llmModelName = v
		} else if v := os.Getenv("ANTHROPIC_MODEL"); v != "" {
			llmModelName = v
		} else if v := os.Getenv("OLLAMA_MODEL"); v != "" {
			llmModelName = v
		} else {
			llmModelName = "gpt-4o" // 默認值
		}
	}

	ctx := context.Background()
	if err := tui.Run(agent, mgr, ctx, llmModelName); err != nil {
		logger.Error("tui run failed", "error", err)
	}
	logger.Info("tui exited")

	// 完成完整的清理過程再退出
	logger.Info("shutting down...")
	mgr.Shutdown()
	os.Exit(0)
}
