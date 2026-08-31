package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"dsc/core"
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

func loadConfig(path string) (*core.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg core.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// getExecutableDir 獲取可執行文件所在目錄的絕對路徑
// resolveWorkspaceRoot 解析統一 workspace 根：
// 僅當配置提供絕對路徑時以其覆蓋（用戶顯式指定工作區）；
// 否則默認以啟動目錄 cwd 為根——在哪个目录启动 dsc，就以哪个目录为工作区
// （对齐 REX/Claude Code 的「以启动目录为工作区」直觉）。相對路徑配置不再
// 参与決定根（避免 ./workspace 把根推到子目錄）。
func resolveWorkspaceRoot(cwd, cfgRoot string) string {
	if filepath.IsAbs(cfgRoot) {
		return filepath.Clean(cfgRoot)
	}
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	return cwd
}

// sandboxPolicyEnv 返回注入各插件进程的沙箱策略档名（DSC_SANDBOX_POLICY）：
// 与 Manager 读取 DSC_SANDBOX 的缺省解析一致，未配置时回退 workspace-write。
func sandboxPolicyEnv() string {
	switch core.ParseSandboxPolicy(os.Getenv("DSC_SANDBOX")) {
	case core.SandboxReadOnly:
		return "read-only"
	case core.SandboxFullAccess:
		return "full-access"
	default:
		return "workspace-write"
	}
}

func getExecutableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}

// statBinaryPath 根據 execDir 与可能的相對/絕對 binaryPath，返回用於 os.Stat 檢查的絕對路徑
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

// runOneTurn 以与 TUI 内部一致的 RunStream 方式运行一组输入（不渲染 TUI），
// 将流式帧直接输出到 stdout，完成后返回退出码（0=成功，1=失败）。
func runOneTurn(agent core.Agent, ctx context.Context, input string) int {
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

// stdinIsRedirected 报告 stdin 是否为非终端（管道/文件重定向），
// 这是多轮 stdin 驱动的判定依据：只有重定向输入才触发多轮，避免终端手动单轮被阻塞。
func stdinIsRedirected() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
}

// runStdinLoop 从 stdin 逐行读取作为后续每一轮输入，逐轮调用 runOneTurn，
// 直到 EOF；会话事件溯源在 agent 内累积，故多轮天然共享上下文。
func runStdinLoop(agent core.Agent, ctx context.Context) int {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	exitCode := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fmt.Fprintf(os.Stderr, "\n>>> %s\n", line)
		if code := runOneTurn(agent, ctx, line); code != 0 {
			exitCode = code
		}
	}
	return exitCode
}

// runInputMode 以非 TUI 模式运行 agent（-input 启动时使用）：
// 首轮跑 -input 传入的文本；随后若 stdin 为重定向输入（管道/文件），
// 则继续逐行驱动多轮，直到 EOF。始终不进入 TUI 事件循环，故 ADMIN API / DEBUGGER
// 端点可在进程存活期间持续观察。
func runInputMode(agent core.Agent, ctx context.Context, input string) int {
	if code := runOneTurn(agent, ctx, input); code != 0 {
		return code
	}
	if !stdinIsRedirected() {
		// 终端手动单轮：跑完 -input 即退出，维持原来的便利行为
		return 0
	}
	return runStdinLoop(agent, ctx)
}

// ptcEnvEnabled 是否由环境变量 DSC_PTC 显式开启 PTC 呈现（判定与 agent 侧 ptcEnabled 一致）。
func ptcEnvEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DSC_PTC"))) {
	case "1", "true", "on", "ptc", "yes":
		return true
	}
	return false
}

func main() {
	// 捕获 panic：打印完整堆栈而非仅值，并以非零码退出——否则 recover 后 main
	// 落到 os.Exit(exitCode) 时 exitCode 仍为默认 0，CI 会把崩溃误判为成功（P2.5）。
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "panic: %v\n%s\n", r, debug.Stack())
			os.Exit(1)
		}
	}()

	// 獲取可執行文件所在目錄的絕對路徑
	execDir, err := getExecutableDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get executable directory: %v\n", err)
		os.Exit(1)
	}

	// dsc setup：交互式配置向导（不加载插件，直接读写 config.yaml）。
	// 检测规则：第一个非 flag 参数（- 开头之外）为 "setup" 时进入向导。
	if isSetupCommand(os.Args[1:]) {
		os.Exit(runSetup(bufio.NewScanner(os.Stdin), os.Stdout,
			filepath.Join(execDir, "config", "config.yaml"),
			filepath.Join(execDir, "plugins")))
	}

	// 啟動目錄（cwd）：默認 workspace 根。用戶在哪个目录启动 dsc，
	// 就以哪个目录为工作区（对齐 REX/Claude Code）；獲取失敗時退化為可執行目錄。
	cwd, err := os.Getwd()
	if err != nil {
		cwd = execDir
	}

	// 解析啟動參數
	logToScreen := false
	logToFile := ""
	mode := "standard"    // 默認標準模式
	inputText := ""       // -input：一次性提示文本（自動化測試入口，不經 TUI）
	debuggerOpen := false // -debugger：開放 /debugger 觀察路由（默認關閉，避免暴露會話隱私）
	adminAddr := ""       // -admin：管理 API 監聽地址（預設取環境變量 DSC_ADMIN_ADDR，缺省 127.0.0.1:9999）
	headless := false     // -headless：精简无头模式，专为 CI 单发（不开 ADMIN/热重载/cron，任务来自 -input）

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
				if nextArg == "minimal" || nextArg == "standard" || nextArg == "creation" || nextArg == "ptc" {
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
		} else if arg == "-headless" {
			// 精简无头模式：单发任务、不开 ADMIN 端口 / 热重载 / cron，专为 CI 脚本
			headless = true
		} else if arg == "-debugger" {
			// 顯式開放 /debugger 觀察路由（含完整會話歷史，屬敏感信息，默認不開放）
			debuggerOpen = true
		} else if arg == "-admin" {
			if i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
				adminAddr = os.Args[i+1]
			}
		}
	}

	// -headless 精简无头模式校验：任务必须来自 -input 且非空白（对齐 harness headless：
	// 缺失/空白任务为用法错误，任何东西都不执行并退出 1）
	if headless {
		if strings.TrimSpace(inputText) == "" {
			fmt.Fprintln(os.Stderr, "headless mode requires -input <task>")
			os.Exit(1)
		}
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "dsc-host",
		Level:  hclog.Info,
		Output: os.Stderr,
	})

	// 初始化 logger 与 coreLogger (根據 logToFile 与 logToScreen 調整)
	var coreLogger hclog.Logger
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
			coreLogger = hclog.New(&hclog.LoggerOptions{
				Name:   "core",
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
			coreLogger = hclog.New(&hclog.LoggerOptions{
				Name:   "core",
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
		coreLogger = hclog.New(&hclog.LoggerOptions{
			Name:   "core",
			Level:  hclog.Info,
			Output: logOutput,
		})
	} else {
		// 無參數時，日誌静默放棄，不作記錄
		logOutput = io.Discard
		logger = hclog.New(&hclog.LoggerOptions{
			Name:   "dsc-host",
			Level:  hclog.NoLevel,
			Output: logOutput,
		})
		coreLogger = hclog.New(&hclog.LoggerOptions{
			Name:   "core",
			Level:  hclog.NoLevel,
			Output: logOutput,
		})
	}

	// 统一把宿主日志与插件日志的扇出接到 ADMIN /logs SSE：`logOutput` 已锁定
	// 最终目的地（os.Stderr / 日志文件 / io.Discard），以其构建一个 LogFanout，
	// 让 logger 与 coreLogger 同时写原始目的地并广播给 /logs 订阅者。这样即便
	// 默认静默模式（io.Discard），也能按需经 ADMIN API 观察运行时日志。
	logFanout := core.NewLogFanout(logOutput)
	logger = hclog.New(&hclog.LoggerOptions{
		Name:   "dsc-host",
		Level:  hclog.Info,
		Output: logFanout,
	})
	coreLogger = hclog.New(&hclog.LoggerOptions{
		Name:   "core",
		Level:  hclog.Info,
		Output: logFanout,
	})

	logger.Info("starting dsc", "mode", mode)

	// 加載 preset 配置文件
	presetPath := filepath.Join(execDir, "config", "presets", fmt.Sprintf("%s.yaml", mode))
	presetCfg, err := loadConfig(presetPath)
	if err != nil {
		// 如果 preset 配置文件不存在或加載失敗，回退到默認 config.yaml
		logger.Info("preset config not found or invalid, using default config", "presetPath", presetPath, "error", err)
		presetCfg = nil
	}

	// 從主配置 config/config.yaml 讀取工作空間根。
	// workspace_root 為統一工作空間根（對齊 DSH ctx.sandboxPolicy 的單一根來源），
	// 同時供 sandbox 判定與注入給各工具插件進程。默認以啟動目錄（cwd）為根——
	// 在哪个目录启动 dsc，就以哪个目录为工作区（对齐 REX/Claude Code）；
	// 僅當配置顯式提供絕對路徑時覆蓋（相對路徑配置不再参与決定根）。
	// 沙箱策略（read-only / workspace-write / full-access）由 TUI /sandbox 命令
	// 运行时切换，见 core.Manager.SetSandboxPolicy。
	mainCfg, err := loadConfig(filepath.Join(execDir, "config", "config.yaml"))
	if err == nil && mainCfg != nil {
		core.WorkspaceRoot = resolveWorkspaceRoot(cwd, mainCfg.WorkspaceRoot)
	}
	logger.Info("workspace root", "root", core.WorkspaceRoot)

	// /settings history 持久化的历史注入编码（config.yaml history_injection）在启动时
	// 下发 agent 子进程：0 未定义（默认不限制，不设）；-1 禁止（不注入）→ agent 0；
	// N>0 启用并注入 N 条 → agent N。
	if mainCfg != nil && mainCfg.HistoryInjection != 0 {
		agentCount := mainCfg.HistoryInjection
		if agentCount == -1 {
			agentCount = 0 // config 禁止 → agent off
		}
		os.Setenv("DSC_HISTORY_INJECTION", strconv.Itoa(agentCount))
		logger.Info("history injection from config", "encoded", mainCfg.HistoryInjection, "agent", agentCount)
	}

	// 放大 go-core GRPCBroker 的连接超时（库默认 5 秒，见 EnvConnTimeout）：
	// 宿主与插件进程同时加载超大本地模型、传输通道被挤占时，接收方收到 ConnInfo
	// 后可能未能及时 Dial，一旦超过一次性窗口即被丢弃且无法重连。统一经环境变量
	// 下发，所有插件子进程（buildEnv 继承宿主环境）同样生效；外部已显式设置时尊重之。
	if os.Getenv("PLUGIN_BROKER_CONN_TIMEOUT") == "" {
		os.Setenv("PLUGIN_BROKER_CONN_TIMEOUT", "5m")
	}
	logger.Info("core broker conn timeout", "timeout", os.Getenv("PLUGIN_BROKER_CONN_TIMEOUT"))

	mgr := core.NewManager(&core.ManagerConfig{
		PluginDir:       filepath.Join(execDir, "plugins"),
		ExecDir:         execDir,
		Handshake:       core.Handshake,
		Logger:          logger,
		PluginLogger:    coreLogger,
		LogFanout:       logFanout,
		DebuggerEnabled: debuggerOpen,
		EnableHotReload: mainCfg != nil && mainCfg.HotReload,
		// PTC 呈现：-mode=ptc 或显式 DSC_PTC 开启（与 agent 侧 ptcEnabled 判定一致）
		PTC: mode == "ptc" || ptcEnvEnabled(),
	})
	defer mgr.Shutdown()
	// 通知 Manager 动态注入/卸载要写回的 config.yaml 路径，
	// 使运行期增删的插件在进程重启后依旧保留
	mgr.SetConfigPath(filepath.Join(execDir, "config", "config.yaml"))

	// fail 記錄錯誤後先清理已加載的插件子進程再退出，避免 os.Exit 跳過 defer 導致殘留孤兒進程
	fail := func(format string, args ...interface{}) {
		logger.Error(fmt.Sprintf(format, args...))
		mgr.Shutdown()
		os.Exit(1)
	}

	// ===== 声明式加载：合并 config.yaml + preset，交给 Manager 按 DependsOn 拓扑加载 =====
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	// 从 config.yaml 收集启用的 LLM 条目与 agent 条目（沿用其 binary_path/env 声明）
	var llmEntries []core.PluginEntry
	var agentEntry *core.PluginEntry
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
		llmEntries = append(llmEntries, core.PluginEntry{
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

	// 组装合并配置：LLM + agent（来自 config.yaml）+ tool/policy/dsc
	// （来自 preset；config.yaml 中启用的 tool/policy/dsc —— 含 install_go_plugin
	// 安装的 —— 亦并入，按名去重、config 优先，使模型安装的插件能跨重启生效）
	merged := assembleMerged(llmEntries, agentEntry, mainCfg, presetCfg, contextWindow, headless, inputText)

	// 注入当前模式 + 工作根 + 沙箱档到所有插件进程（DSC_MODE）：tool-lua-host 据此限制
	// 「插件创造」仅在创造模式（creation）下允许。
	injectRuntimeEnv(merged, mode, core.WorkspaceRoot, sandboxPolicyEnv())

	// 声明式加载：Manager 内做依赖拓扑排序 + PENDING + 聚合 Tool 服务 + 一次性 RegisterServices。
	// 失败则 P2 自愈：把 config.yaml 与 preset 各自备份当前（坏）版、分别还原各自最近正常
	// 备份，再用还原后的配置重建插件集重试一次；仍失败才退出。
	mainConfigPath := filepath.Join(execDir, "config", "config.yaml")
	pluginsDir := filepath.Join(execDir, "plugins")
	pluginsSnap := pluginsDir + "-backup"
	loadErr := mgr.LoadFromConfig(merged)
	if loadErr != nil {
		// 先锁定最近「正常」备份（避免把下面刚备份的坏版当成正常版回读），
		// 再把当前坏版各自备份留档，最后分别还原各自最近正常备份续启
		latestCfg, _ := core.LatestGoodBackup(mainConfigPath)
		latestPreset, _ := core.LatestGoodBackup(presetPath)
		badCfg, _ := core.BackupGoodFile(mainConfigPath, logger)
		badPreset, _ := core.BackupGoodFile(presetPath, logger)
		recovered := ""
		if latestCfg != "" {
			if err := core.RestoreGoodFile(mainConfigPath, latestCfg, logger); err == nil {
				recovered = latestCfg
			}
		}
		if latestPreset != "" {
			if err := core.RestoreGoodFile(presetPath, latestPreset, logger); err == nil && recovered == "" {
				recovered = latestPreset
			}
		}
		// 插件目录与配置无法对齐（如二进制缺失/损坏）时，从「上次正常」快照回拷合并
		if err := core.RestorePluginsDir(pluginsSnap, pluginsDir, logger); err != nil {
			logger.Warn("plugins dir restore skipped (no snapshot?)", "err", err)
		}
		if recovered != "" {
			// 用还原后的 config.yaml / preset 重建插件集并重试
			if newMain, _ := loadConfig(mainConfigPath); newMain != nil {
				newPreset := presetCfg
				if p, _ := loadConfig(presetPath); p != nil {
					newPreset = p
				}
				merged = assemblePluginSet(newMain, newPreset, contextWindow, headless, inputText)
				injectRuntimeEnv(merged, mode, core.WorkspaceRoot, sandboxPolicyEnv())
				logger.Warn("启动加载失败，已还原最近正常配置并重试（降级模式）",
					"cause", loadErr, "recovered", recovered, "badConfig", badCfg, "badPreset", badPreset)
				if err := mgr.LoadFromConfig(merged); err != nil {
					fail("failed to load plugins declaratively (even after config restore): %v", err)
				}
				loadErr = nil
			}
		}
		if loadErr != nil {
			fail("failed to load plugins declaratively: %v", loadErr)
		}
	}

	// 成功启动：把已生效的 config.yaml 与当前 preset 各自独立备份一次（P2 自愈基础——
	// 之后若被改坏，可分别还原各自最近正常版本续启）
	if _, err := core.BackupGoodFile(mainConfigPath, logger); err != nil {
		logger.Warn("failed to backup config after successful start", "err", err)
	}
	if _, err := core.BackupGoodFile(presetPath, logger); err != nil {
		logger.Warn("failed to backup preset after successful start", "err", err)
	}
	// P3：维护 plugins 目录「上次正常」快照（二进制大，仅当有新内容才刷新）
	if done, err := core.BackupPluginsDir(pluginsDir, pluginsSnap, logger); err != nil {
		logger.Warn("failed to snapshot plugins dir", "err", err)
	} else if done {
		logger.Info("plugins dir snapshot refreshed", "snap", pluginsSnap)
	}

	// 后台监听/调度仅在常规模式启用；-headless 精简无头模式不开端口、不起常驻轮询，
	// 对齐 harness headless「进程只存活于单发、不留任何后台」的契约。
	if !headless {
		// 版本化二进制自动热重载（config.yaml hot_reload: true 时启用）：fsnotify + 周期扫描
		if err := mgr.StartHotReloadWatcher(); err != nil {
			logger.Warn("failed to start hot-reload watcher", "err", err)
		}

		// 启动 cron 定时任务调度器（失败仅告警，不阻塞主流程）
		if err := mgr.StartCron(); err != nil {
			logger.Warn("failed to start cron scheduler", "err", err)
		} else {
			logger.Info("cron scheduler started")
		}

		// 启动管理 API：监听地址取 -admin 旗标，未指定则回退环境变量 DSC_ADMIN_ADDR，
		// 再默认绑定回环地址 127.0.0.1:9999（避免局域网内任意主机在未配置
		// DSC_ADMIN_TOKEN 时经 /plugins/load 加载任意二进制触发 RCE；需要远程
		// 管理时显式用 -admin :9999 并配置 token）。
		if adminAddr == "" {
			adminAddr = os.Getenv("DSC_ADMIN_ADDR")
		}
		if adminAddr == "" {
			adminAddr = "127.0.0.1:9999"
		}
		mgr.StartAdmin(adminAddr)
		logger.Info("admin api started", "addr", adminAddr)
	}

	// 获取 Agent 并运行（经事件包装：RunStream 回合完成时广播 agent/status，
	// 供 notify 等程序性插件订阅；对齐 DSH agent-loop 原生发 agent/status）
	agentName := mgr.GetMainAgentName()
	if agentName == "" {
		agentName = "agent-react-loop"
	}
	agent, ok := mgr.EventAgent()
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

		// 回合完成音效宽限：notify 等程序性通知插件在 agent 回合完成时异步播放完成音效
		// （success/error，约 0.29s）。-input 单发回合结束后宿主随即回收插件子进程
		// （Windows 上为强杀，不跑 defer），可能导致音效被截断。已加载通知插件且非
		// headless（CI 单发无需报声）时，短暂宽限让音效播完再关闭；headless 保持快速退出。
		if !headless && mgr.HasPlugin("dsc-notify") {
			const completionSoundGrace = 800 * time.Millisecond
			logger.Info("draining completion sound", "grace", completionSoundGrace.String())
			time.Sleep(completionSoundGrace)
		}
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
