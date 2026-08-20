package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"dsc/plugin"
	"dsc/proto"
	"dsc/tui"
	"github.com/hashicorp/go-hclog"
	"gopkg.in/yaml.v3"
	"google.golang.org/grpc"
)

// defaultContextWindow 未配置且探测失败时使用的默认上下文窗口大小：128K（按 1024 计）
const defaultContextWindow = 128 * 1024

// probeContextWindow 探测 LLAMACPP（或兼容 OpenAI 的服务）的上下文窗口大小。
// 通过 GET {baseURL}/models 读取首条模型的 meta.n_ctx（LLAMACPP 提供），
// 失败或未提供时返回 0，由调用方回退到配置值或默认 128K。
func probeContextWindow(baseURL string) int {
	u := strings.TrimRight(baseURL, "/") + "/models"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0
	}
	var payload struct {
		Data []struct {
			Meta struct {
				Nctx int `json:"n_ctx"`
			} `json:"meta"`
			MaxModelLen int `json:"max_model_len"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0
	}
	if len(payload.Data) == 0 {
		return 0
	}
	if payload.Data[0].Meta.Nctx > 0 {
		return payload.Data[0].Meta.Nctx
	}
	if payload.Data[0].MaxModelLen > 0 {
		return payload.Data[0].MaxModelLen
	}
	return 0
}

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
	resp, err := llm.Chat(ctx, messages, tools, int(req.MaxTokens))
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
			Usage:        plugin.UsageToProto(item.Usage),
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

// getExecutableDir 獲取可執行文件所在目錄的絕對路徑
func getExecutableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}

// runInputMode 以一次性模式运行 agent（-input 启动时使用）：
// 与 TUI 内部一致地走 RunStream（保持数据交互环节一致），但不渲染 TUI，
// 将流式帧直接输出到 stdout，完成后返回退出码（0=成功，1=失败）。
// 代理循环仅运行一次（一个输入），方便完成自动化测试后程序自然退出。
func runInputMode(agent plugin.Agent, ctx context.Context, input string) int {
	ch, err := agent.RunStream(ctx, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 1
	}
	exitCode := 0
	for frame := range ch {
		switch frame.Status {
		case "streaming":
			// 模型文本增量，直接输出
			fmt.Print(frame.Output)
		case "tool":
			// 工具调用提示（如 [调用工具: shell]），原样输出
			fmt.Print(frame.Output)
		case "success":
			// 一轮完成；usage 仅提示到 stderr，不污染 stdout 结果
			if frame.Usage != nil && frame.Usage.TotalTokens > 0 {
				fmt.Fprintf(os.Stderr, "\n[已用 %d tokens]\n", frame.Usage.TotalTokens)
			}
		case "error":
			if frame.Error != "" {
				fmt.Fprintf(os.Stderr, "\n错误: %s\n", frame.Error)
			}
			exitCode = 1
		case "reasoning":
			fmt.Fprintf(os.Stderr, "\\n[REASONING]>%s", frame.Reasoning)
		}
	}
	return exitCode
}

func main() {
	// 捕獲 panic 並輸出到 stderr，以便在日誌靜默時能調試啟動失敗原因
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "panic recovered: %v\n", r)
		}
	}()

	// 獲取可執行文件所在目錄的絕對路徑
	execDir, err := getExecutableDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get executable directory: %v\n", err)
		os.Exit(1)
	}

	// 解析啟動參數
	logToScreen := false
	logToFile := ""
	mode := "standard" // 默認標準模式
	inputText := ""    // -input：一次性提示文本（自動化測試入口，不經 TUI）

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
		} else if arg == "-input" {
			// -input 後跟提示文本作為參數值（以 - 開頭的視為缺失，避免吞掉後續選項）
			if i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
				inputText = os.Args[i+1]
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
	presetPath := filepath.Join(execDir, "config", "presets", fmt.Sprintf("%s.yaml", mode))
	presetCfg, err := loadConfig(presetPath)
	if err != nil {
		// 如果 preset 配置文件不存在或加載失敗，回退到默認 config.yaml
		logger.Info("preset config not found or invalid, using default config", "presetPath", presetPath, "error", err)
		presetCfg = nil
	}

	mgr := plugin.NewManager(&plugin.ManagerConfig{
		PluginDir:    filepath.Join(execDir, "plugins"),
		ExecDir:      execDir,
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

	// 從配置中獲取 LLM 插件的 binary_path 和 env
	llmBinary := ""
	llmEnv := map[string]string{}

	// 嘗試從 presetCfg 中獲取 LLM 配置
	var llmEntry *plugin.PluginEntry
	if presetCfg != nil {
		for _, entry := range presetCfg.Plugins {
			if entry.Type == "llm" && entry.Name == llmName && entry.Enabled {
				llmEntry = &entry
				break
			}
		}
	}

	// 如果 presetCfg 中沒有找到，嘗試從 mainCfg 中獲取
	if llmEntry == nil {
		if mainCfg, err := loadConfig(filepath.Join(execDir, "config", "config.yaml")); err == nil {
			for _, entry := range mainCfg.Plugins {
				if entry.Type == "llm" && entry.Name == llmName && entry.Enabled {
					llmEntry = &entry
					break
				}
			}
		}
	}

	if llmEntry != nil {
		llmBinary = llmEntry.BinaryPath
		llmEnv = llmEntry.Env
	}

	// 如果配置中沒有指定 binary_path，則使用默認路徑規則
	if llmBinary == "" {
		llmBinary = filepath.Join(execDir, "plugins", "llm-"+llmName, "llm-"+llmName+ext)
	}

	// 檢查 LLM 二進制文件是否存在
	if _, err := os.Stat(llmBinary); os.IsNotExist(err) {
		fail("LLM binary not found for provider %q at %q", llmName, llmBinary)
	}

	if err := mgr.LoadLLM(llmName, llmBinary, llmEnv); err != nil {
		fail("failed to load LLM %s: %v", llmName, err)
	}
	logger.Info("llm loaded", "name", llmName)

	// 計算上下文窗口容量（token 數）：
	// 優先取配置值 → 探測 LLAMACPP /v1/models 的 n_ctx → 默認 128K×1024
	contextWindow := 0
	if mainCfg, err := loadConfig(filepath.Join(execDir, "config", "config.yaml")); err == nil && mainCfg.ContextWindow > 0 {
		contextWindow = mainCfg.ContextWindow
	}
	if contextWindow == 0 {
		if baseURL := llmEnv["OPENAI_BASE_URL"]; baseURL != "" {
			contextWindow = probeContextWindow(baseURL)
			if contextWindow > 0 {
				logger.Info("context window probed from llm server", "window", contextWindow)
			}
		}
	}
	if contextWindow == 0 {
		contextWindow = defaultContextWindow
	}
	logger.Info("context window", "window", contextWindow)

	// 2. 加載 Agent 插件（把上下文窗口容量傳給 react-loop，供其做 80% 自動壓縮；
	//    -input 模式下同時傳入單輪模式標記，讓代理循環僅執行一次）
	agentBinary := filepath.Join(execDir, "plugins", "react-loop", "react-loop"+ext)
	agentEnv := map[string]string{
		"DSC_CONTEXT_WINDOW": strconv.Itoa(contextWindow),
	}
	// 把预设 persona（"你是一個…助手" 身份句）傳給 react-loop；無則為空、走官方默認
	if presetCfg != nil && presetCfg.Persona != "" {
		agentEnv["DSC_PRESET_PERSONA"] = presetCfg.Persona
	}
	if inputText != "" {
		agentEnv["DSC_SINGLE_TURN"] = "1"
	}
	broker, llmServiceID, err := mgr.LoadAgentAndGetBroker("react-agent", agentBinary, agentEnv)
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

	exitCode := 0
	if inputText != "" {
		// -input：一次性模式，不显示 TUI，完成后自然退出
		logger.Info("running in input mode", "input", inputText)
		exitCode = runInputMode(agent, ctx, inputText)
		logger.Info("input mode finished", "exitCode", exitCode)
	} else {
		if err := tui.Run(agent, mgr, ctx, llmModelName, mode, contextWindow); err != nil {
			logger.Error("tui run failed", "error", err)
			exitCode = 1
		}
		logger.Info("tui exited")
	}

	// 完成完整的清理過程再退出
	logger.Info("shutting down...")
	mgr.Shutdown()
	os.Exit(exitCode)
}
