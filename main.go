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
	"dsc/tui"
	"github.com/hashicorp/go-hclog"
	"gopkg.in/yaml.v3"
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

// statBinaryPath 根據 execDir 和可能的相對/絕對 binaryPath，返回用於 os.Stat 檢查的絕對路徑
func statBinaryPath(execDir, cfgPath string, defaultRel string) string {
	p := cfgPath
	if p == "" {
		p = filepath.Join(execDir, defaultRel)
	} else {
		if !filepath.IsAbs(p) {
			p = filepath.Join(execDir, p)
		}
	}
	return p
}

// loadBinaryPath 返回用於傳遞給插件管理器的路徑（通常是配置文件中的相對路徑或默認相對路徑）
func loadBinaryPath(cfgPath string, defaultRel string) string {
	if cfgPath != "" {
		return cfgPath
	}
	return defaultRel
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

	// 從主配置 config/config.yaml 讀取工作空間保護狀態
	// 默認啟用工作區保護，防止路徑遍歷攻擊
	plugin.WorkspaceProtectionEnabled = true
	mainCfg, err := loadConfig(filepath.Join(execDir, "config", "config.yaml"))
	if err == nil && mainCfg != nil && !mainCfg.WorkspaceProtectionEnabled {
		plugin.WorkspaceProtectionEnabled = false
	}
	logger.Info("workspace protection enabled", "enabled", plugin.WorkspaceProtectionEnabled)

	mgr := plugin.NewManager(&plugin.ManagerConfig{
		PluginDir:    filepath.Join(execDir, "plugins"),
		ExecDir:      execDir,
		Handshake:    plugin.Handshake,
		Logger:       logger,
		PluginLogger: pluginLogger,
	})
	defer mgr.Shutdown()
	// 通知 Manager 动态注入/卸载要写回的 config.yaml 路径，
	// 使运行期增删的插件在进程重启后依旧保留（第 4 步）
	mgr.SetConfigPath(filepath.Join(execDir, "config", "config.yaml"))

	// fail 記錄錯誤後先清理已加載的插件子進程再退出，避免 os.Exit 跳過 defer 導致殘留孤兒進程
	fail := func(format string, args ...interface{}) {
		logger.Error(fmt.Sprintf(format, args...))
		mgr.Shutdown()
		os.Exit(1)
	}

	// ===== 声明式加载（第 2 步）：合并 config.yaml + preset，交给 Manager 按 DependsOn 拓扑加载 =====
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	// 从 config.yaml 收集启用的 LLM 条目与 agent 条目（沿用其 binary_path/env 声明）
	var llmEntries []plugin.PluginEntry
	var agentEntry *plugin.PluginEntry
	if mainCfg != nil {
		for i := range mainCfg.Plugins {
			e := mainCfg.Plugins[i]
			if !e.Enabled {
				continue
			}
			switch e.Type {
			case "llm":
				llmEntries = append(llmEntries, e)
			case "agent":
				if agentEntry == nil {
					agentEntry = &mainCfg.Plugins[i]
				}
			}
		}
	}

	// 若 config.yaml 未声明任何启用的 LLM，则按 环境变量 → DefaultLLM → openai 回退构造默认条目
	if len(llmEntries) == 0 {
		llmName := os.Getenv("LLM_PROVIDER")
		if llmName == "" && mainCfg != nil {
			llmName = mainCfg.DefaultLLM
		}
		if llmName == "" {
			llmName = "openai"
		}
		llmEntries = append(llmEntries, plugin.PluginEntry{
			Name:       llmName,
			Type:       "llm",
			Enabled:    true,
			BinaryPath: loadBinaryPath("", "./plugins/llm-"+llmName+"/llm-"+llmName+ext),
		})
	}

	// 确定“活跃 LLM”用于探测上下文窗口与展示模型名：agent.depends_on.llm → LLM_PROVIDER → DefaultLLM → 首个启用条目
	activeLLMName := ""
	if agentEntry != nil && agentEntry.DependsOn != nil && agentEntry.DependsOn.LLM != "" {
		activeLLMName = agentEntry.DependsOn.LLM
	}
	if activeLLMName == "" {
		if v := os.Getenv("LLM_PROVIDER"); v != "" {
			activeLLMName = v
		} else if mainCfg != nil && mainCfg.DefaultLLM != "" {
			activeLLMName = mainCfg.DefaultLLM
		}
	}
	if activeLLMName == "" {
		activeLLMName = llmEntries[0].Name
	}

	activeLLMBinary := ""
	activeLLMEnv := map[string]string{}
	for _, e := range llmEntries {
		if e.Name == activeLLMName {
			activeLLMEnv = e.Env
			rel := e.BinaryPath
			if rel == "" {
				rel = "./plugins/llm-" + activeLLMName + "/llm-" + activeLLMName + ext
			}
			activeLLMBinary = statBinaryPath(execDir, rel, "./plugins/llm-"+activeLLMName+"/llm-"+activeLLMName+ext)
			break
		}
	}
	if activeLLMBinary == "" {
		activeLLMBinary = statBinaryPath(execDir, "", "./plugins/llm-"+activeLLMName+"/llm-"+activeLLMName+ext)
	}
	if _, err := os.Stat(activeLLMBinary); os.IsNotExist(err) {
		fail("LLM binary not found for provider %q at %q", activeLLMName, activeLLMBinary)
	}

	// 上下文窗口容量（token 数）：配置值 → 探测 LLAMACPP /v1/models 的 n_ctx → 默认 128K×1024
	contextWindow := 0
	if mainCfg != nil && mainCfg.ContextWindow > 0 {
		contextWindow = mainCfg.ContextWindow
	}
	if contextWindow == 0 {
		if baseURL := activeLLMEnv["OPENAI_BASE_URL"]; baseURL != "" {
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

	// 组装合并配置：LLM + agent（来自 config.yaml）+ tool/policy（来自 preset）
	merged := &plugin.Config{}
	merged.Plugins = append(merged.Plugins, llmEntries...)
	if agentEntry == nil {
		// config.yaml 未声明 agent，用默认 agent-react-loop
		merged.Plugins = append(merged.Plugins, plugin.PluginEntry{
			Name:       "agent-react-loop",
			Type:       "agent",
			Enabled:    true,
			BinaryPath: loadBinaryPath("", "./plugins/agent-react-loop/agent-react-loop"+ext),
		})
	} else {
		// 为 agent 注入运行参数（上下文窗口、persona、单轮标记），再并入其声明 env
		agentEnv := map[string]string{
			"DSC_CONTEXT_WINDOW": strconv.Itoa(contextWindow),
		}
		if presetCfg != nil && presetCfg.Persona != "" {
			agentEnv["DSC_PRESET_PERSONA"] = presetCfg.Persona
		}
		if inputText != "" {
			agentEnv["DSC_SINGLE_TURN"] = "1"
		}
		for k, v := range agentEntry.Env {
			agentEnv[k] = v
		}
		agentEntry.Env = agentEnv
		merged.Plugins = append(merged.Plugins, *agentEntry)
	}
	if presetCfg != nil {
		for _, e := range presetCfg.Plugins {
			if e.Enabled && (e.Type == "tool" || e.Type == "policy") {
				merged.Plugins = append(merged.Plugins, e)
			}
		}
	}

	// 声明式加载：Manager 内做依赖拓扑排序 + PENDING + 聚合 Tool 服务 + 一次性 RegisterServices，
	// 取代原先 Main 手工编排 LLM→Agent→Tools 顺序与两段式依赖注入
	if err := mgr.LoadFromConfig(merged); err != nil {
		fail("failed to load plugins declaratively: %v", err)
	}

	// 启动管理 API
	adminAddr := os.Getenv("DSC_ADMIN_ADDR")
	if adminAddr == "" {
		adminAddr = ":9999"
	}
	mgr.StartAdmin(adminAddr)
	logger.Info("admin api started", "addr", adminAddr)

	// 获取 Agent 并运行
	agentName := mgr.GetMainAgentName()
	if agentName == "" {
		agentName = "agent-react-loop"
	}
	agent, ok := mgr.GetAgent(agentName)
	if !ok {
		fail("agent %s not found after loading", agentName)
	}

	// 从配置提取 LLM 模型名称用于 TUI 展示
	llmModelName := "Unknown"
	for _, e := range llmEntries {
		if e.Name == activeLLMName {
			if v, ok := e.Env["ANTHROPIC_MODEL"]; ok {
				llmModelName = v
			} else if v, ok := e.Env["OPENAI_MODEL"]; ok {
				llmModelName = v
			} else if v, ok := e.Env["OLLAMA_MODEL"]; ok {
				llmModelName = v
			}
			break
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
			llmModelName = "Agentic-Model" // 默認值
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
