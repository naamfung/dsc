package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dsc/cron"
	"dsc/jobs"
	"dsc/proto"
	"dsc/proto/metadata"
	"dsc/session"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/hashicorp/go-version"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"
)

// Manager 插件管理器
type Manager struct {
	mu              sync.RWMutex
	clients         map[string]*goplugin.Client // 插件名 -> 客戶端
	plugins         map[string]DSCPlugin        // 插件名 -> DSC業務接口
	agents          map[string]Agent            // 插件名 -> Agent接口
	llms            map[string]LLMProvider      // 插件名 -> LLMProvider接口
	typeMap         map[string]string           // 插件名 -> 類型 ("dsc", "agent", "llm", "tool")
	agentServiceIDs map[string]uint32           // agent name -> serviceID
	config          *ManagerConfig
	toolRegistry    *ToolRegistry // 新增
	logger          hclog.Logger
	pluginMetadata  map[string]*metadata.PluginInfo // 插件名 -> 元數據
	pluginLogger    hclog.Logger                    // logger for go-plugin internal logs

	// 新增字段用於 Broker 和服務 ID 管理
	broker        *goplugin.GRPCBroker // 統一的 broker，由 Agent 提供
	mainAgentName string               // 主 Agent 名稱
	// agentToolServiceID 是挂载在 broker 上的“聚合 Tool 服务”ID：汇总所有工具插件注册表的统一入口，
	// 一次性注入给 Agent（RegisterServices.ToolServiceId），热重载时沿用同一 ID。
	agentToolServiceID  uint32
	llmServiceIDs       map[string]uint32                  // llm name -> serviceID
	llmOrder            []string                           // LLM provider 加载顺序（多 provider 路由的 fallback 序）
	agentLLMServiceID   uint32                             // 聚合 LLM 服务 ID（多 provider 路由，agent 一次性注入）
	agentLLMName        string                             // agent 声明的 primary LLM（路由优先）
	toolServiceIDs      map[string]uint32                  // tool plugin name -> serviceID
	pluginToolNames     map[string][]string                // tool plugin name -> list of tool names it provides
	toolNameToServiceID map[string]uint32                  // tool name -> serviceID
	toolClients         map[string]proto.ToolServiceClient // tool plugin name -> 插件 ToolService 客户端（供 ListContext 聚合）
	// 互通机制 3：插件钩子回调客户端（toolHookClients，按加载顺序 toolHookOrder）。
	// 宿主在工具流水线（BeforeTool/AfterTool）与事件广播（OnEvent）时回调插件，
	// 使插件 A 无需通知主程序/其他插件即可钩子式改变插件 B 的工具行为。
	toolHookClients map[string]proto.PluginHookServiceClient
	toolHookOrder   []string
	states          map[string]*RuntimeState // 插件名 -> 运行时状态快照

	// 动态注入插件相关字段：
	// configPath 动态注入/卸载写回的 config.yaml 路径（config 始终为运行态唯一事实来源）。
	// agentEntries  记录已声明的 agent 条目（含 DependsOn），供 PENDING agent 再激活时解析依赖。
	// pendingEntries 记录依赖未满足、等待后续注入的插件条目（provider 未拉起，agent 已拉起）。
	configPath     string
	agentEntries   map[string]PluginEntry
	pendingEntries map[string]PluginEntry

	// 事件总线：插件生命周期状态迁移事件的订阅者表。
	// eventsMu 独立于 m.mu，避免状态机持锁发布事件时与订阅操作死锁。
	eventsMu    sync.RWMutex
	subscribers map[int]chan PluginEvent
	nextSubID   int

	// events 通用事件分发总线（emit/parallel/serial/bail/waterfall）：
	// 供工具执行流水线等宿主内扩展点使用，与上面的生命周期推送通道正交。
	events *EventBus

	// cronScheduler cron 定时任务调度器（StartCron 启动，Shutdown 停止）。
	cronScheduler *cron.Scheduler
	// 用户评审通道：userQuestionProvider 为 UI provider（TUI 注册）；
	// userQuestionsServiceID 为挂载在 broker 上的 UserQuestionsService ID（注入 agent）。
	userQuestionProvider   UserQuestionProvider
	userQuestionsServiceID uint32
	// pluginNotifyServiceID 挂载在 broker 上的 PluginNotifyService ID
	// （互通机制 2：插件进程经它向宿主事件总线发布事件）。
	pluginNotifyServiceID uint32

	// policyClients 已加载 policy 插件的策略服务客户端（按插件名），
	// 由桥接逻辑包装为工具流水线监听器（替代旁路）。
	policyClients map[string]proto.FsObservationPolicyServiceClient
	// policyOff policy 桥接监听器的移除函数（按插件名），卸载时一并撤销。
	policyOff map[string][]func()

	// sandboxPolicyVal 运行时沙箱策略（atomic，支持 TUI /sandbox 命令动态切换）。
	sandboxPolicyVal atomic.Int32

	// stopHooks 对称清理 hook：插件名 -> 按注册顺序执行的清理函数序列。
	stopHooks map[string][]func() error

	// jobs 后台任务注册表（对齐 DSH jobs v1 种子）：workflow 后台运行等
	// 长任务经 Registry.Start 启动，模型用 job_output/job_list/job_kill 查询管理。
	jobs *jobs.Registry
}

type ManagerConfig struct {
	PluginDir    string
	ExecDir      string // 可執行文件所在目錄，用作插件子進程的工作目錄
	Handshake    goplugin.HandshakeConfig
	Logger       hclog.Logger
	PluginLogger hclog.Logger // logger for go-plugin internal logs (e.g., plugin process exited)
}

func NewManager(cfg *ManagerConfig) *Manager {
	logger := cfg.Logger
	if logger == nil {
		logger = hclog.New(&hclog.LoggerOptions{
			Name:  "manager",
			Level: hclog.Info,
		})
	}
	pluginLogger := cfg.PluginLogger
	if pluginLogger == nil {
		pluginLogger = hclog.New(&hclog.LoggerOptions{
			Name:  "plugin",
			Level: hclog.Info,
		})
	}
	m := &Manager{
		clients:             make(map[string]*goplugin.Client),
		plugins:             make(map[string]DSCPlugin),
		agents:              make(map[string]Agent),
		llms:                make(map[string]LLMProvider),
		typeMap:             make(map[string]string),
		agentServiceIDs:     make(map[string]uint32),
		config:              cfg,
		toolRegistry:        NewToolRegistry(),
		pluginMetadata:      make(map[string]*metadata.PluginInfo),
		logger:              logger,
		broker:              nil,
		mainAgentName:       "",
		llmServiceIDs:       make(map[string]uint32),
		toolServiceIDs:      make(map[string]uint32),
		pluginToolNames:     make(map[string][]string),
		toolNameToServiceID: make(map[string]uint32),
		toolClients:         make(map[string]proto.ToolServiceClient),
		toolHookClients:     make(map[string]proto.PluginHookServiceClient),
		states:              make(map[string]*RuntimeState),
		pluginLogger:        pluginLogger,
		subscribers:         make(map[int]chan PluginEvent),
		stopHooks:           make(map[string][]func() error),
		agentEntries:        make(map[string]PluginEntry),
		pendingEntries:      make(map[string]PluginEntry),
		events:              NewEventBus(),
		policyClients:       make(map[string]proto.FsObservationPolicyServiceClient),
		policyOff:           make(map[string][]func()),
		jobs:                jobs.NewRegistry(),
	}
	// 註冊內置工具（現已遷移至獨立插件 tool-str-replace-editor）
	// 後續可註冊更多工具
	// 内建 subagent 工具（宿主侧子代理，委派任务给独立小循环）
	_ = m.toolRegistry.Register(&subagentTool{m: m})
	// 内建 workflow 工具（宿主侧 JS 编排脚本，可扇出 subagent；支持后台运行）
	_ = m.toolRegistry.Register(&workflowTool{m: m})
	// 内建 job 工具族（后台任务查询/取消，对齐 DSH tool-jobs：job_output/job_list/job_kill）
	_ = m.toolRegistry.Register(&jobTool{m: m, name: "job_output"})
	_ = m.toolRegistry.Register(&jobTool{m: m, name: "job_list"})
	_ = m.toolRegistry.Register(&jobTool{m: m, name: "job_kill"})
	// 后台任务完成 → 宿主事件总线（通用送达：TUI 唤醒、web/novelforge 等插件订阅）
	m.jobs.OnJobDone(func(s jobs.JobSnapshot) {
		m.events.Emit(JobDoneEvent, EventContext{Data: s})
	})
	// 宿主事件 → 插件广播（互通机制 3：插件经 PluginHookService.OnEvent 订阅）
	m.events.OnAny(func(ctx EventContext) (any, error) {
		m.broadcastEventToPlugins(ctx.Name, ctx.Data)
		return nil, nil
	})
	// spill：超长工具结果外置（阈值 4000 字符，目录可经 DSC_SPILL_DIR 配置）；
	// post-execute 策略 + read_spill 取回工具
	spillDir := os.Getenv("DSC_SPILL_DIR")
	if spillDir == "" {
		if cfg.ExecDir != "" {
			spillDir = filepath.Join(cfg.ExecDir, "spill")
		} else {
			// 嘗試獲取可執行文件所在目錄
			if execPath, err := os.Executable(); err == nil {
				spillDir = filepath.Join(filepath.Dir(execPath), "spill")
			} else {
				logger.Warn("spill store: cannot determine executable dir, using default 'spill' dir", "error", err)
				spillDir = "spill"
			}
		}
	}
	if store, err := NewSpillStore(spillDir); err == nil {
		_ = m.toolRegistry.Register(&readSpillTool{store: store})
		m.events.OnWaterfall(EventToolPostExecute, spillLargeResult(store, 4000))
	} else {
		logger.Warn("spill store unavailable", "error", err)
	}
	// sandbox：进程效应策略层（DSC_SANDBOX: full/workspace/readonly，缺省 workspace），
	// pre-execute fail-closed 拦截写操作；运行时可用 SetSandboxPolicy 动态切换
	m.sandboxPolicyVal.Store(int32(ParseSandboxPolicy(os.Getenv("DSC_SANDBOX"))))
	m.events.OnWaterfall(EventToolPreExecute, sandboxPolicy(m.GetSandboxPolicy))
	// LLM 请求默认带退避重试（最多 2 次，300ms 起指数退避）；流中途失败不重试
	m.events.OnWaterfall(EventLLMRequest, LLMRetryListener(2, 300*time.Millisecond))
	return m
}

// trackStateLocked 在加载入口预置 Spawned 状态（需已持有 m.mu）。
// 若插件已有更靠后的状态（如热重载中途失败重试），不强制回退。
func (m *Manager) trackStateLocked(name, typ string) {
	st, ok := m.states[name]
	if !ok {
		m.states[name] = &RuntimeState{Type: typ}
		st = m.states[name]
	}
	if st.State == "" {
		st.State = StateSpawned
		st.UpdatedAt = time.Now()
	}
}

// transitionLocked 迁移插件状态并落盘快照（需已持有 m.mu）。
// 记录 to 时的错误信息；非法迁移会告警，便于暴露流程漏步。
func (m *Manager) transitionLocked(name string, to PluginState, lastErr string) {
	st, ok := m.states[name]
	if !ok {
		st = &RuntimeState{}
		m.states[name] = st
	}
	old := st.State
	if lastErr != "" {
		st.LastError = lastErr
	}
	if to == StateFailed && lastErr == "" && st.LastError != "" {
		// 保留上一次错误信息
	}
	st.State = to
	st.UpdatedAt = time.Now()

	if old == to {
		return
	}
	if validPluginTransition(old, to) {
		m.logger.Info("plugin state transition", "name", name, "from", old, "to", to)
	} else if old != "" {
		m.logger.Warn("invalid plugin state transition", "name", name, "from", old, "to", to)
	}
	m.publishEventLocked(PluginEvent{
		Name:  name,
		Type:  st.Type,
		From:  old,
		To:    to,
		Error: st.LastError,
		Time:  time.Now(),
	})
}

// markPendingLocked 把插件置为待办（PENDING）：依赖未满足，暂不对外服务（需已持有 m.mu）。
// 对应 DSH 的 PENDING，等待依赖注入后就绪。进程尚未拉起的插件直接预置 PENDING 快照；
// 已拉起的（如 agent）则从当前态迁入 PENDING。
func (m *Manager) markPendingLocked(name, typ string, reason string) {
	st, ok := m.states[name]
	if !ok {
		m.states[name] = &RuntimeState{Type: typ, State: StatePending, UpdatedAt: time.Now(), LastError: reason}
		m.logger.Info("plugin marked pending", "name", name, "reason", reason)
		m.publishEventLocked(PluginEvent{
			Name:  name,
			Type:  typ,
			To:    StatePending,
			Error: reason,
			Time:  time.Now(),
		})
		return
	}
	st.Type = typ
	m.transitionLocked(name, StatePending, reason)
}

// isPendingLocked 判断名为 name 的插件当前是否处于 PENDING（需已持有 m.mu）。
func (m *Manager) isPendingLocked(name string) bool {
	st, ok := m.states[name]
	return ok && st.State == StatePending
}

// markDisposedLocked 統一卸載終態：Stopping -> Disposed（需已持有 m.mu）。
func (m *Manager) markDisposedLocked(name string) {
	m.transitionLocked(name, StateDisposed, "")
}

// monitorExit 监听子进程退出事件：仅当退出进程仍是当前注册的 client 且非主动卸载时，
// 才把插件标记为 Failed，使崩溃可观测而非静默消失。热重载换进程后旧 client 的调用会被忽略。
// 当前 go-plugin 版本仅暴露 Exited() bool（无等待通道），故采用轻量轮询。
func (m *Manager) monitorExit(name string, client *goplugin.Client) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if !client.Exited() {
			continue
		}
		m.mu.Lock()
		// 该进程已被卸载/替换，退出本次监控
		if m.clients[name] != client {
			m.mu.Unlock()
			return
		}
		st := m.states[name]
		unexpected := st == nil || (st.State != StateDisposed && st.State != StateFailed)
		if unexpected {
			m.transitionLocked(name, StateFailed, "plugin process exited unexpectedly")
		}
		m.mu.Unlock()
		return
	}
}

// Load 加載一個插件（啟動子進程）
func (m *Manager) Load(name string, binaryPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 跨平台處理二進制路徑
	binaryPath = normalizeBinaryPath(binaryPath)

	// 如果已加載，先卸載
	if _, exists := m.plugins[name]; exists {
		m.unloadLocked(name)
	}
	m.trackStateLocked(name, "dsc")

	// 創建插件客戶端
	cmd := exec.Command(binaryPath)
	if m.config.ExecDir != "" {
		cmd.Dir = m.config.ExecDir
	}
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"dsc_plugin": &DSCPluginGRPC{},
		},
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	// 建立 RPC 連接
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return fmt.Errorf("failed to connect to plugin: %w", err)
	}
	m.transitionLocked(name, StateConnecting, "")

	// 獲取插件實例
	raw, err := rpcClient.Dispense("dsc_plugin")
	if err != nil {
		client.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return fmt.Errorf("failed to dispense plugin: %w", err)
	}

	impl, ok := raw.(DSCPlugin)
	if !ok {
		client.Kill()
		m.transitionLocked(name, StateFailed, "plugin does not implement DSCPlugin interface")
		return fmt.Errorf("plugin does not implement DSCPlugin interface")
	}

	m.clients[name] = client
	m.plugins[name] = impl
	m.typeMap[name] = "dsc"
	m.transitionLocked(name, StateReady, "")
	go m.monitorExit(name, client)

	m.logger.Info("plugin loaded", "name", name)
	return nil
}

// Unload 卸載插件（殺死子進程，釋放資源）
func (m *Manager) Unload(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unloadLocked(name)
}

func (m *Manager) unloadLocked(name string) error {
	if client, exists := m.clients[name]; exists {
		m.transitionLocked(name, StateUnloading, "")
		m.runStopHooksLocked(name) // 对称清理：先撤销工具/优雅关闭，再终止进程
		delete(m.clients, name)    // 先摘除，令退出监控忽略该进程
		client.Kill()              // 殺死插件子進程
		delete(m.plugins, name)
		delete(m.typeMap, name)
		m.markDisposedLocked(name)
		m.logger.Info("plugin unloaded", "name", name)
		return nil
	}
	return fmt.Errorf("plugin '%s' not found", name)
}

// Get 獲取已加載的插件實例
func (m *Manager) Get(name string) (DSCPlugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[name]
	return p, ok
}

// List 列出所有已加載插件
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.plugins))
	for name := range m.plugins {
		names = append(names, name)
	}
	return names
}

// LoadAgent 加載一個 Agent 插件（啟動子進程）
func (m *Manager) LoadAgent(name string, binaryPath string, serviceID uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if serviceID == 0 {
		return fmt.Errorf("serviceID must not be 0")
	}

	// 跨平台處理二進制路徑
	binaryPath = normalizeBinaryPath(binaryPath)

	// 如果已加載，先卸載
	if _, exists := m.agents[name]; exists {
		m.unloadAgentLocked(name)
	}
	m.trackStateLocked(name, "agent")

	// 構建命令，加入 -llm-service-id 參數
	cmdArgs := []string{"-llm-service-id", strconv.FormatUint(uint64(serviceID), 10)}
	cmd := exec.Command(binaryPath, cmdArgs...)
	if m.config.ExecDir != "" {
		cmd.Dir = m.config.ExecDir
	}

	// 創建插件客戶端
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"agent": &AgentGRPCPlugin{},
		},
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	// 建立 RPC 連接
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return fmt.Errorf("failed to connect to agent plugin: %w", err)
	}
	m.transitionLocked(name, StateConnecting, "")

	// 獲取插件實例
	raw, err := rpcClient.Dispense("agent")
	if err != nil {
		client.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return fmt.Errorf("failed to dispense agent plugin: %w", err)
	}

	impl, ok := raw.(Agent)
	if !ok {
		client.Kill()
		m.transitionLocked(name, StateFailed, "plugin does not implement Agent interface")
		return fmt.Errorf("plugin does not implement Agent interface")
	}

	m.clients[name] = client
	m.agents[name] = impl
	m.typeMap[name] = "agent"
	m.transitionLocked(name, StateReady, "")
	go m.monitorExit(name, client)

	m.logger.Info("agent plugin loaded", "name", name)
	return nil
}

// UnloadAgent 卸載 Agent 插件（殺死子進程，釋放資源）
func (m *Manager) UnloadAgent(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unloadAgentLocked(name)
}

func (m *Manager) unloadAgentLocked(name string) error {
	if client, exists := m.clients[name]; exists {
		m.transitionLocked(name, StateUnloading, "")
		m.runStopHooksLocked(name) // 对称清理：先优雅关闭 agent，再终止进程
		delete(m.clients, name)    // 先摘除，令退出监控忽略该进程
		client.Kill()              // 殺死插件子進程
		delete(m.agents, name)
		delete(m.typeMap, name)
		delete(m.agentServiceIDs, name)
		m.markDisposedLocked(name)
		m.logger.Info("agent plugin unloaded", "name", name)
		return nil
	}
	return fmt.Errorf("agent plugin '%s' not found", name)
}

// GetAgent 獲取已加載的 Agent 實例
func (m *Manager) GetAgent(name string) (Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.agents[name]
	return p, ok
}

// LoadAgentAndGetBroker 加載 Agent 插件，並返回其 GRPCBroker 和 serviceID 以供宿主註冊服務。
// env 為傳遞給 Agent 插件子進程的自定義環境變量（與宿主環境合併，插件值優先）。
// 該方法會將 Agent 納入 Manager 管理，支持後續熱重載。
func (m *Manager) LoadAgentAndGetBroker(name, binaryPath string, env map[string]string) (*goplugin.GRPCBroker, uint32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 校驗插件目錄命名規範
	dirName := getPluginDirectoryName(binaryPath)
	if err := validatePluginDirectoryName("agent", dirName); err != nil {
		return nil, 0, fmt.Errorf("invalid Agent plugin directory name '%s': %w", dirName, err)
	}

	// 如果已加載，先卸載
	if _, exists := m.agents[name]; exists {
		m.unloadAgentLocked(name)
	}
	m.trackStateLocked(name, "agent")

	// 創建插件客戶端（先不傳遞 serviceID，稍後通過 broker 生成）
	cmd := exec.Command(binaryPath)
	if m.config.ExecDir != "" {
		cmd.Dir = m.config.ExecDir
	}
	if len(env) > 0 {
		cmd.Env = buildEnv(env)
	}
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"agent": &AgentGRPCPlugin{},
		},
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return nil, 0, fmt.Errorf("failed to connect to agent plugin: %w", err)
	}
	m.transitionLocked(name, StateConnecting, "")

	grpcClient, ok := rpcClient.(*goplugin.GRPCClient)
	if !ok {
		client.Kill()
		m.transitionLocked(name, StateFailed, "agent plugin is not a gRPC client")
		return nil, 0, fmt.Errorf("agent plugin is not a gRPC client")
	}

	broker := grpcClient.Broker()
	m.broker = broker // 供後續 LoadToolsAndPoliciesFromConfig 等使用
	serviceID := broker.NextId()

	// 獲取 Agent 實例（用於調用 RPC）
	raw, err := rpcClient.Dispense("agent")
	if err != nil {
		client.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return nil, 0, fmt.Errorf("failed to dispense agent plugin: %w", err)
	}

	impl, ok := raw.(Agent)
	if !ok {
		client.Kill()
		m.transitionLocked(name, StateFailed, "plugin does not implement Agent interface")
		return nil, 0, fmt.Errorf("plugin does not implement Agent interface")
	}

	m.clients[name] = client
	m.agents[name] = impl
	m.typeMap[name] = "agent"
	m.agentServiceIDs[name] = serviceID
	// 注册对称清理 hook：卸载/热重载时先优雅关闭 agent，再终止进程
	ai := impl
	m.addStopHookLocked(name, func() error {
		return ai.Shutdown(context.Background(), false)
	})
	m.transitionLocked(name, StateActive, "")
	go m.monitorExit(name, client)

	m.logger.Info("agent plugin loaded", "name", name, "serviceID", serviceID)
	return broker, serviceID, nil
}

// LoadLLM 加載 LLM 插件；env 為傳遞給插件子進程的自定義環境變量（與宿主環境合併，插件值優先）
func (m *Manager) LoadLLM(name string, binaryPath string, env map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.loadLLMEntryLocked(PluginEntry{
		Name:       name,
		Type:       "llm",
		BinaryPath: binaryPath,
		Env:        env,
	})
	return err
}

// loadLLMEntryLocked 加載 LLM 插件並存儲 provider（需已持有 m.mu）。
// 供 LoadFromConfig 在声明式加载流程中复用，避免重复加锁。
func (m *Manager) loadLLMEntryLocked(entry PluginEntry) (LLMProvider, error) {
	binaryPath := entry.BinaryPath
	if binaryPath == "" {
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}
		binaryPath = fmt.Sprintf("./plugins/%s/%s%s", entry.Name, entry.Name, ext)
	}
	binaryPath = normalizeBinaryPath(binaryPath)

	name := entry.Name
	// 校驗插件目錄命名規範
	dirName := getPluginDirectoryName(binaryPath)
	if err := validatePluginDirectoryName("llm", dirName); err != nil {
		return nil, fmt.Errorf("invalid LLM plugin directory name '%s': %w", dirName, err)
	}

	// 如果已加載，先卸載
	if _, exists := m.llms[name]; exists {
		m.unloadLLMLocked(name)
	}
	m.trackStateLocked(name, "llm")

	// 創建插件客戶端
	cmd := exec.Command(binaryPath)
	if m.config.ExecDir != "" {
		cmd.Dir = m.config.ExecDir
	}
	if len(entry.Env) > 0 {
		cmd.Env = buildEnv(entry.Env)
	}
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"llm": &LLMGRPCPlugin{},
		},
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	// 建立 RPC 連接
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return nil, fmt.Errorf("failed to connect to LLM plugin: %w", err)
	}
	m.transitionLocked(name, StateConnecting, "")

	// 獲取插件實例
	raw, err := rpcClient.Dispense("llm")
	if err != nil {
		client.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return nil, fmt.Errorf("failed to dispense LLM plugin: %w", err)
	}

	impl, ok := raw.(LLMProvider)
	if !ok {
		client.Kill()
		m.transitionLocked(name, StateFailed, "plugin does not implement LLMProvider interface")
		return nil, fmt.Errorf("plugin does not implement LLMProvider interface")
	}

	m.clients[name] = client
	m.llms[name] = impl
	m.llmOrder = append(m.llmOrder, name) // 记录加载顺序（去重由加载前 unload 保证）
	m.typeMap[name] = "llm"
	m.transitionLocked(name, StateReady, "")
	go m.monitorExit(name, client)

	m.logger.Info("llm plugin loaded", "name", name)
	return impl, nil
}

// GetLLM 獲取已加載的 LLM 實例
func (m *Manager) GetLLM(name string) (LLMProvider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.llms[name]
	return p, ok
}

// UnloadLLM 卸載 LLM 插件
func (m *Manager) UnloadLLM(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unloadLLMLocked(name)
}

func (m *Manager) unloadLLMLocked(name string) error {
	if client, exists := m.clients[name]; exists {
		m.transitionLocked(name, StateUnloading, "")
		m.runStopHooksLocked(name) // 统一对称清理入口（LLM 暂未注册 hook，保持空跑幂等）
		delete(m.clients, name)    // 先摘除，令退出监控忽略该进程
		client.Kill()              // 殺死插件子進程
		delete(m.llms, name)
		delete(m.typeMap, name)
		m.markDisposedLocked(name)
		m.logger.Info("llm plugin unloaded", "name", name)
		return nil
	}
	return fmt.Errorf("LLM plugin '%s' not found", name)
}

// HotReload 熱重載：卸載舊版本，加載新版本
// 支持 DSCPlugin、Agent、LLM 三種類型的插件，根據 typeMap 記錄的類型自動判斷
func (m *Manager) HotReload(name string, newBinaryPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	typ, ok := m.typeMap[name]
	if !ok {
		return fmt.Errorf("plugin '%s' not found (no registered type)", name)
	}
	switch typ {
	case "dsc":
		return m.hotReloadPluginLocked(name, newBinaryPath)
	case "agent":
		return m.hotReloadAgentLocked(name, newBinaryPath)
	case "llm":
		return m.hotReloadLLMLocked(name, newBinaryPath)
	default:
		return fmt.Errorf("unknown plugin type '%s' for name '%s'", typ, name)
	}
}

// hotReloadPluginLocked 內部重載 DSCPlugin（需已持有鎖）
func (m *Manager) hotReloadPluginLocked(name, newBinaryPath string) error {
	// 1. 建立新 client 並驗證
	cmd := exec.Command(newBinaryPath)
	if m.config.ExecDir != "" {
		cmd.Dir = m.config.ExecDir
	}
	newClient := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"dsc_plugin": &DSCPluginGRPC{},
		},
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	rpcClient, err := newClient.Client()
	if err != nil {
		newClient.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return fmt.Errorf("new plugin connection failed: %w", err)
	}
	m.transitionLocked(name, StateConnecting, "")

	raw, err := rpcClient.Dispense("dsc_plugin")
	if err != nil {
		newClient.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return fmt.Errorf("dispense new plugin failed: %w", err)
	}

	newImpl, ok := raw.(DSCPlugin)
	if !ok {
		newClient.Kill()
		m.transitionLocked(name, StateFailed, "new plugin does not implement DSCPlugin interface")
		return fmt.Errorf("new plugin does not implement DSCPlugin interface")
	}

	// 3. 先对称清理旧实例，再更新映射（令旧进程的退出监控不再生效）
	m.runStopHooksLocked(name)
	oldClient := m.clients[name]
	m.clients[name] = newClient
	m.plugins[name] = newImpl
	m.typeMap[name] = "dsc"
	if oldClient != nil {
		oldClient.Kill()
	}
	m.transitionLocked(name, StateReady, "")
	go m.monitorExit(name, newClient)

	m.logger.Info("plugin hot-reloaded", "name", name)
	return nil
}

// hotReloadAgentLocked 內部重載 Agent（需已持有鎖）
func (m *Manager) hotReloadAgentLocked(name, newBinaryPath string) error {
	oldServiceID, ok := m.agentServiceIDs[name]
	if !ok {
		return fmt.Errorf("no serviceID recorded for agent %s", name)
	}

	// 1. 建立新 client 並驗證
	cmd := exec.Command(newBinaryPath)
	if m.config.ExecDir != "" {
		cmd.Dir = m.config.ExecDir
	}
	newClient := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"agent": &AgentGRPCPlugin{},
		},
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	rpcClient, err := newClient.Client()
	if err != nil {
		newClient.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return fmt.Errorf("new agent plugin connection failed: %w", err)
	}
	m.transitionLocked(name, StateConnecting, "")

	grpcClient, ok := rpcClient.(*goplugin.GRPCClient)
	if !ok {
		newClient.Kill()
		m.transitionLocked(name, StateFailed, "new agent plugin is not a gRPC client")
		return fmt.Errorf("new agent plugin is not a gRPC client")
	}

	// 透過 RPC 重新注冊服务ID（tool 沿用聚合 Tool 服务；LLM 走聚合多 provider 路由）
	agentClient := proto.NewAgentServiceClient(grpcClient.Conn)
	_, err = agentClient.RegisterServices(context.Background(), &proto.RegisterServicesRequest{LlmServiceId: m.agentLLMServiceID, ToolServiceId: m.agentToolServiceID})
	if err != nil {
		newClient.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return fmt.Errorf("failed to register service IDs on agent: %w", err)
	}
	// 重新注入用户评审通道 serviceID（同一 broker，沿用挂载的服务）
	if m.userQuestionsServiceID != 0 {
		_, _ = agentClient.SetUserQuestionsService(context.Background(), &proto.SetUserQuestionsServiceRequest{ServiceId: m.userQuestionsServiceID})
	}

	raw, err := rpcClient.Dispense("agent")
	if err != nil {
		newClient.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return fmt.Errorf("dispense new agent plugin failed: %w", err)
	}

	newImpl, ok := raw.(Agent)
	if !ok {
		newClient.Kill()
		m.transitionLocked(name, StateFailed, "new agent plugin does not implement Agent interface")
		return fmt.Errorf("new agent plugin does not implement Agent interface")
	}

	// 3. 先对称清理旧 agent，再更新映射（旧进程退出监控随之失效）
	m.runStopHooksLocked(name) // 优雅关闭旧实例
	oldClient := m.clients[name]
	m.clients[name] = newClient
	m.agents[name] = newImpl
	m.typeMap[name] = "agent"
	if oldClient != nil {
		oldClient.Kill()
	}
	// 为新实例注册对称清理 hook
	ni := newImpl
	m.addStopHookLocked(name, func() error {
		return ni.Shutdown(context.Background(), false)
	})
	m.transitionLocked(name, StateActive, "")
	go m.monitorExit(name, newClient)

	m.logger.Info("agent plugin hot-reloaded", "name", name, "serviceID", oldServiceID)
	return nil
}

// hotReloadLLMLocked 內部重載 LLM（需已持有鎖）
func (m *Manager) hotReloadLLMLocked(name, newBinaryPath string) error {
	// 1. 建立新 client 並驗證
	cmd := exec.Command(newBinaryPath)
	if m.config.ExecDir != "" {
		cmd.Dir = m.config.ExecDir
	}
	newClient := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"llm": &LLMGRPCPlugin{},
		},
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	rpcClient, err := newClient.Client()
	if err != nil {
		newClient.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return fmt.Errorf("new LLM plugin connection failed: %w", err)
	}
	m.transitionLocked(name, StateConnecting, "")

	raw, err := rpcClient.Dispense("llm")
	if err != nil {
		newClient.Kill()
		m.transitionLocked(name, StateFailed, err.Error())
		return fmt.Errorf("dispense new LLM plugin failed: %w", err)
	}

	newImpl, ok := raw.(LLMProvider)
	if !ok {
		newClient.Kill()
		m.transitionLocked(name, StateFailed, "new LLM plugin does not implement LLMProvider interface")
		return fmt.Errorf("new LLM plugin does not implement LLMProvider interface")
	}

	// 3. 先对称清理旧实例，再更新映射（旧进程退出监控随之失效）
	m.runStopHooksLocked(name)
	oldClient := m.clients[name]
	m.clients[name] = newClient
	m.llms[name] = newImpl
	m.typeMap[name] = "llm"
	if oldClient != nil {
		oldClient.Kill()
	}
	m.transitionLocked(name, StateActive, "")
	go m.monitorExit(name, newClient)

	m.logger.Info("LLM plugin hot-reloaded", "name", name)
	return nil
}

// ListLLMs 列出所有已加載的 LLM 插件
func (m *Manager) ListLLMs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.llms))
	for name := range m.llms {
		names = append(names, name)
	}
	return names
}

// Shutdown 關閉所有插件
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCronLocked() // 先停调度，避免新任务触发时插件已退出
	for name, client := range m.clients {
		m.transitionLocked(name, StateUnloading, "")
		delete(m.clients, name)    // 先摘除，令退出监控忽略
		m.runStopHooksLocked(name) // 对称清理后再终止进程
		client.Kill()
		m.markDisposedLocked(name)
		m.logger.Info("plugin killed", "name", name)
	}
	m.clients = make(map[string]*goplugin.Client)
	m.plugins = make(map[string]DSCPlugin)
	m.agents = make(map[string]Agent)
	m.llms = make(map[string]LLMProvider)
	m.typeMap = make(map[string]string)
	m.agentServiceIDs = make(map[string]uint32)
	m.toolNameToServiceID = make(map[string]uint32)
	m.pluginMetadata = make(map[string]*metadata.PluginInfo)
	m.states = make(map[string]*RuntimeState)
	m.stopHooks = make(map[string][]func() error)
	m.agentEntries = make(map[string]PluginEntry)
	m.pendingEntries = make(map[string]PluginEntry)
}

// ListAgents 列出所有已加載的 Agent 插件
func (m *Manager) ListAgents() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.agents))
	for name := range m.agents {
		names = append(names, name)
	}
	return names
}

// GetToolRegistry 暴露工具註冊表
func (m *Manager) GetToolRegistry() *ToolRegistry {
	return m.toolRegistry
}

// SessionSummary 会话列表的宿主侧摘要（TUI 展示用）。
type SessionSummary struct {
	ID      string `json:"id"`
	Events  int    `json:"events"`
	Preview string `json:"preview"`
}

// ListSessions 列出多会话 store 中的会话（读 ExecDir/sessions 目录）。
func (m *Manager) ListSessions() ([]SessionSummary, error) {
	st, err := m.sessionStore()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	infos, err := st.List()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	out := make([]SessionSummary, len(infos))
	for i, info := range infos {
		out[i] = SessionSummary{ID: info.ID, Events: info.Events, Preview: info.Preview}
	}
	return out, nil
}

// CreateSession 在多会话 store 中新建会话并返回其 id（TUI /session new 用）。
func (m *Manager) CreateSession() (string, error) {
	st, err := m.sessionStore()
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	sess, err := st.Create()
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return sess.ID(), nil
}

// DeleteSession 删除指定会话（按文件删除；删除后 agent 侧需切走当前会话，
// 由调用方保证不删除正在使用的会话）。
func (m *Manager) DeleteSession(id string) error {
	st, err := m.sessionStore()
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	if err := st.Delete(id); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// sessionStore 打开/创建多会话 store（ExecDir/sessions）。
func (m *Manager) sessionStore() (*session.Store, error) {
	dir := filepath.Join(m.config.ExecDir, "sessions")
	return session.NewStore(dir)
}

// ExportSession 导出指定会话为 Markdown transcript（ExecDir/exports/<id>.md），
// 返回导出文件路径。
func (m *Manager) ExportSession(id string) (string, error) {
	st, err := m.sessionStore()
	if err != nil {
		return "", fmt.Errorf("export session: %w", err)
	}
	sess, err := st.Load(id)
	if err != nil {
		return "", fmt.Errorf("export session: %w", err)
	}
	if sess == nil {
		return "", fmt.Errorf("export session: session %q not found", id)
	}
	dir := filepath.Join(m.config.ExecDir, "exports")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("export session: %w", err)
	}
	path := filepath.Join(dir, id+".md")
	if err := os.WriteFile(path, []byte(sess.ExportTranscript()), 0644); err != nil {
		return "", fmt.Errorf("export session: %w", err)
	}
	return path, nil
}

// ListContext 聚合所有已加載工具插件貢獻的上下文片段（如技能索引），
// 供 agent 拼接到 system prompt。未實現 ListContext 的舊插件返回 Unimplemented，跳過即可。
func (m *Manager) ListContext(ctx context.Context) (string, error) {
	m.mu.Lock()
	clients := make([]proto.ToolServiceClient, 0, len(m.toolClients))
	for _, c := range m.toolClients {
		clients = append(clients, c)
	}
	m.mu.Unlock()

	var parts []string
	for _, c := range clients {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		resp, err := c.ListContext(ctx, &proto.ListContextRequest{})
		cancel()
		if err != nil {
			// 舊插件未實現 ListContext（Unimplemented）或暂时不可用，跳过
			continue
		}
		if content := strings.TrimSpace(resp.GetContent()); content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

// UnloadPlugin 卸載插件
func (m *Manager) UnloadPlugin(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, exists := m.clients[name]
	if !exists {
		// 未加载的插件：若只是 PENDING 待办，同样从待办/配置中清除
		if _, pending := m.pendingEntries[name]; pending {
			delete(m.pendingEntries, name)
			if err := m.persistRemovalLocked(name); err != nil {
				m.logger.Warn("persist removal failed", "name", name, "error", err)
			}
			return nil
		}
		return fmt.Errorf("plugin '%s' not found", name)
	}

	// 对称清理：执行已注册的 stopHook（tool 插件撤销其工具、agent 优雅关闭）；
	// llm 的 serviceID 映射不依赖 hook，此处保留
	if m.typeMap[name] == "llm" {
		delete(m.llmServiceIDs, name)
	}
	m.transitionLocked(name, StateUnloading, "")
	m.runStopHooksLocked(name)

	client.Kill() // 殺死插件子進程
	delete(m.clients, name)
	delete(m.plugins, name)
	delete(m.agents, name)
	delete(m.llms, name)
	delete(m.typeMap, name)
	delete(m.pluginMetadata, name)
	delete(m.pendingEntries, name)
	delete(m.agentEntries, name)
	// 撤销 policy 桥接的流水线监听器
	for _, off := range m.policyOff[name] {
		off()
	}
	delete(m.policyOff, name)
	delete(m.policyClients, name)
	m.markDisposedLocked(name)

	// 持久化：从 config.yaml 移除该插件声明，使运行态与重启态一致
	if err := m.persistRemovalLocked(name); err != nil {
		m.logger.Warn("persist removal failed", "name", name, "error", err)
	}

	m.logger.Info("plugin unloaded", "name", name)
	return nil
}

// PluginInfoSummary 插件信息摘要
type PluginInfoSummary struct {
	Name    string      `json:"name"`
	Type    string      `json:"type"`
	Version string      `json:"version"`
	Enabled bool        `json:"enabled"`
	State   PluginState `json:"state"`
}

// ListPlugins 返回所有已加載插件的列表，并携带运行时状态。
// 除当前已注册/已加载的插件外，也会带上已卸载(disposed/failed)的终态插件，便于观测最近失败或下线的插件。
func (m *Manager) ListPlugins() []PluginInfoSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	seen := make(map[string]bool, len(m.pluginMetadata)+len(m.typeMap)+len(m.states))
	var plugins []PluginInfoSummary

	add := func(name, typ, version string, enabled bool, state PluginState) {
		if seen[name] {
			return
		}
		seen[name] = true
		plugins = append(plugins, PluginInfoSummary{
			Name:    name,
			Type:    typ,
			Version: version,
			Enabled: enabled,
			State:   state,
		})
	}

	for name, info := range m.pluginMetadata {
		st := m.states[name]
		state := StateActive
		if st != nil {
			state = st.State
		}
		add(name, info.Type, info.Version, true, state)
	}
	for name, typ := range m.typeMap {
		if seen[name] {
			continue
		}
		st := m.states[name]
		state := StateReady
		if st != nil {
			state = st.State
		}
		add(name, typ, "unknown", true, state)
	}
	// 已卸载/失败的终态插件：保留观测，不视为启用中
	for name, st := range m.states {
		if seen[name] {
			continue
		}
		enabled := st.State == StateFailed // 失败仍属目标集合，可重试
		add(name, st.Type, "unknown", enabled, st.State)
	}
	return plugins
}

// GetPluginState 获取指定插件的当前运行时状态。
func (m *Manager) GetPluginState(name string) (RuntimeState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st, ok := m.states[name]
	if !ok {
		return RuntimeState{}, false
	}
	return *st, true
}

// GetPluginMetadata 獲取特定插件的元數據
func (m *Manager) GetPluginMetadata(name string) (*metadata.PluginInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info, exists := m.pluginMetadata[name]
	return info, exists
}

// GetMainAgentName 獲取主 Agent 名稱
func (m *Manager) GetMainAgentName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mainAgentName
}

// LoadPlugin 动态加载/注入插件（供 Admin /plugins/load 使用）。
// 委托给 inject.go 的 injectionEntryLocked，做完整的端到端注入：
// 依赖判定（未满足→PENDING）、声明持久化（写回 config.yaml）、等待中的 PENDING 提升与 agent 再激活。
func (m *Manager) LoadPlugin(entry PluginEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.injectionEntryLocked(entry)
}

// SetConfigPath 设置动态注入/卸载要写回的 config.yaml 路径。
func (m *Manager) SetConfigPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configPath = path
}

// llmProxyServer 實現 LLMService，轉發到 LLMProvider
type llmProxyServer struct {
	proto.UnimplementedLLMServiceServer
	client proto.LLMServiceClient
}

func (s *llmProxyServer) Chat(ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	return s.client.Chat(ctx, req)
}

// ChatStream 将上游 LLM 插件的流式响应逐帧转发给下游（react-loop agent）
func (s *llmProxyServer) ChatStream(req *proto.ChatRequest, stream proto.LLMService_ChatStreamServer) error {
	cli, err := s.client.ChatStream(stream.Context(), req)
	if err != nil {
		return err
	}
	for {
		msg, err := cli.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// [DEBUG] 打印轉發的 ChatStreamResponse
		if msg.Reasoning != "" || msg.Content != "" {
			fmt.Fprintf(os.Stderr, "[LLM-PROXY-DEBUG] Send: Content=%q, Reasoning=%q, FinishReason=%q\n", msg.Content, msg.Reasoning, msg.FinishReason)
		}
		if err := stream.Send(msg); err != nil {
			return err
		}
	}
}

func (s *llmProxyServer) Name(ctx context.Context, req *proto.NameRequest) (*proto.NameResponse, error) {
	return s.client.Name(ctx, req)
}

func (s *llmProxyServer) Version(ctx context.Context, req *proto.VersionRequest) (*proto.VersionResponse, error) {
	return s.client.Version(ctx, req)
}

func (s *llmProxyServer) HealthCheck(ctx context.Context, req *proto.HealthCheckRequest) (*proto.HealthCheckResponse, error) {
	return s.client.HealthCheck(ctx, req)
}

// toolProxyServer 實現 ToolService，轉發到 ToolServiceClient
type toolProxyServer struct {
	proto.UnimplementedToolServiceServer
	client proto.ToolServiceClient
}

func (s *toolProxyServer) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest) (*proto.ExecuteToolResponse, error) {
	return s.client.ExecuteTool(ctx, req)
}

func (s *toolProxyServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	return s.client.ListTools(ctx, req)
}

func (s *toolProxyServer) ListContext(ctx context.Context, req *proto.ListContextRequest) (*proto.ListContextResponse, error) {
	return s.client.ListContext(ctx, req)
}

// normalizeBinaryPath 跨平台處理二進制路徑
// 統一將路徑中的「\」轉換為「/」
// Windows 系統：確保有 .exe 後綴
// 非 Windows 系統：移除 .exe 後綴
func normalizeBinaryPath(path string) string {
	// 統一將反斜槓轉換為正斜槓
	path = strings.ReplaceAll(path, `\\`, "/")
	path = strings.ReplaceAll(path, "\\", "/")

	pathLower := strings.ToLower(path)
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(pathLower, ".exe") {
			return path + ".exe"
		}
	} else {
		if strings.HasSuffix(pathLower, ".exe") {
			// 移除後綴 .exe
			return path[:len(path)-4]
		}
	}
	return path
}

// getPluginDirectoryName 從二進制路徑中提取插件目錄名
func getPluginDirectoryName(binaryPath string) string {
	dir := filepath.Dir(binaryPath)
	baseDir := filepath.Base(dir)
	if baseDir == "plugins" || baseDir == "plugin" {
		// 如果 dir 是 .../plugins，說明 binaryPath 是 ./plugins/<binaryName>.exe 或 plugins/<binaryName>.exe
		// 非 Windows 系統下 binaryPath 已無後綴，Windows 系統下為 .exe 後綴
		basename := filepath.Base(binaryPath)
		basename = strings.TrimSuffix(basename, ".exe")
		return basename
	}
	return baseDir
}

// validatePluginDirectoryName 校驗插件目錄名是否符合規範：<類別>-<名稱>
func validatePluginDirectoryName(pluginType, dirName string) error {
	prefix := ""
	switch pluginType {
	case "llm":
		prefix = "llm-"
	case "agent":
		prefix = "agent-"
	case "tool":
		prefix = "tool-"
	case "policy":
		prefix = "policy-"
	case "dsc":
		prefix = "dsc-"
	default:
		return fmt.Errorf("unknown plugin type '%s', cannot validate directory name '%s'", pluginType, dirName)
	}

	// 檢查 dirName 是否以 prefix 開頭，並且後續部分只包含字母、數字、連字號和下劃線
	if !strings.HasPrefix(dirName, prefix) {
		return fmt.Errorf("plugin directory name '%s' does not match expected prefix '%s' for type '%s'", dirName, prefix, pluginType)
	}

	suffix := strings.TrimPrefix(dirName, prefix)
	if suffix == "" {
		return fmt.Errorf("plugin directory name '%s' is incomplete for type '%s'", dirName, pluginType)
	}

	// 檢查 suffix 是否只包含字母、數字、連字號和下劃線
	matched, err := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, suffix)
	if err != nil {
		return fmt.Errorf("error validating plugin directory name '%s': %v", dirName, err)
	}
	if !matched {
		return fmt.Errorf("plugin directory name '%s' contains invalid characters for type '%s'", dirName, pluginType)
	}

	return nil
}

// CheckCircularDependencies 檢查插件配置中是否存在環形依賴關係
func CheckCircularDependencies(entries []PluginEntry) error {
	// 構建插件地圖和依賴圖
	pluginNodes := make(map[string]bool)
	dependencies := make(map[string][]string)

	for _, entry := range entries {
		if !entry.Enabled {
			continue
		}
		pluginNodes[entry.Name] = true
		dependencies[entry.Name] = []string{}

		if entry.DependsOn != nil {
			if entry.DependsOn.LLM != "" {
				dependencies[entry.Name] = append(dependencies[entry.Name], entry.DependsOn.LLM)
			}
			for _, tool := range entry.DependsOn.Tools {
				dependencies[entry.Name] = append(dependencies[entry.Name], tool)
			}
		}
	}

	// DFS 狀態：0 = unvisited, 1 = visiting, 2 = visited
	state := make(map[string]int)
	var cyclePlugins []string

	var dfs func(node string, path []string) bool
	dfs = func(node string, path []string) bool {
		if state[node] == 1 {
			// 發現環形依賴，找出環中的插件
			cycleStart := -1
			for i := len(path) - 1; i >= 0; i-- {
				if path[i] == node {
					cycleStart = i
					break
				}
			}
			if cycleStart != -1 {
				cyclePlugins = path[cycleStart:]
			}
			return true
		}
		if state[node] == 2 {
			return false
		}
		state[node] = 1
		path = append(path, node)

		for _, dep := range dependencies[node] {
			// 只檢查存在的插件節點
			if pluginNodes[dep] {
				if dfs(dep, path) {
					return true
				}
			}
		}

		state[node] = 2
		path = path[:len(path)-1]
		return false
	}

	for pluginName := range pluginNodes {
		if state[pluginName] == 0 {
			if dfs(pluginName, nil) {
				if len(cyclePlugins) > 0 {
					return fmt.Errorf("circular dependency detected among plugins: %v", cyclePlugins)
				}
				return fmt.Errorf("circular dependency detected among plugins")
			}
		}
	}

	return nil
}

// LoadFromConfig 声明式加载所有插件：
// 复用配置中的 DependsOn/Type，在 Manager 内做依赖拓扑排序，取代原先由 Main 宿主手工编排加载序、
// 手工两段式注入依赖的做法。流程：
//  1. Agent 作为 broker 提供者优先拉起进程（获取 broker），但先不激活（状态 Ready/PENDING）；
//  2. 其余 LLM/Tool/Policy 按 DependsOn 拓扑排序加载（LLM 经 loadLLMEntryLocked 原生加载后，
//     再由 serveLLMProviderLocked 挂载为 broker 上的 gRPC 服务；Tool/Policy 走 loadPluginWithBroker）；
//     依赖未满足的 provider 置为 PENDING 并跳过，而非硬失败；
//  3. 依 agent 的 DependsOn 解析 LLM serviceID 与「聚合 Tool 服务」ID，一次性 RegisterServices 注入并置为 ACTIVE；
//     若 agent 声明的 LLM 依赖缺失，则退回 PENDING 等待后续注入。
func (m *Manager) LoadFromConfig(cfg *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 檢查環形依賴關係
	if err := CheckCircularDependencies(cfg.Plugins); err != nil {
		return fmt.Errorf("circular dependency detected in plugin configuration: %w", err)
	}

	// 整体已声明（启用）的插件名，用于判定依赖指向的是否为本配置声明的插件
	declared := make(map[string]bool)
	for _, e := range cfg.Plugins {
		if e.Enabled {
			declared[e.Name] = true
		}
	}

	// 分離 Agent（broker 提供者）與 provider（LLM/Tool/Policy）
	var agentEntry *PluginEntry
	var providerEntries []PluginEntry
	for i := range cfg.Plugins {
		entry := cfg.Plugins[i]
		if !entry.Enabled {
			continue
		}
		if entry.Type == "agent" {
			if agentEntry == nil {
				agentEntry = &cfg.Plugins[i]
			} else {
				m.logger.Warn("multiple agent plugins found, using first one", "first", agentEntry.Name, "ignored", entry.Name)
			}
		} else if entry.Type == "llm" || entry.Type == "tool" || entry.Type == "policy" {
			providerEntries = append(providerEntries, entry)
		}
	}

	if agentEntry == nil {
		return fmt.Errorf("no agent plugin found in config")
	}

	// 先拉起 agent 进程获取 broker（代理依赖 LLM/Tool 的注入，激活放到 provider 就绪之后）
	broker, _, err := m.loadAgentAndGetBroker(*agentEntry)
	if err != nil {
		return fmt.Errorf("failed to load agent: %w", err)
	}
	m.broker = broker
	m.mainAgentName = agentEntry.Name

	// 互通服务先行挂载：聚合 LLM / 插件通知 / 用户评审通道的 serviceID 需在
	// provider 加载前确定，pluginEnv 才会把 ID 注入插件进程——工具插件（如
	// novelforge）加载时即可经互通机制 1/2 复用宿主 LLM 与事件总线。聚合
	// LLM 服务在请求时动态路由 provider，提前挂载安全。
	hasLLM := false
	llmID := uint32(0)
	toolID := uint32(0)
	if agentEntry.DependsOn != nil && agentEntry.DependsOn.LLM != "" {
		if id, err := m.serveAggregateLLMLocked(agentEntry.DependsOn.LLM); err == nil {
			llmID = id
		} else {
			m.logger.Warn("aggregate llm service unavailable", "error", err)
		}
	}
	if _, err := m.servePluginNotifyLocked(); err != nil {
		m.logger.Warn("plugin notify service unavailable", "error", err)
	}
	if uqID, err := m.serveUserQuestionsLocked(); err == nil {
		if agent, ok := m.agents[agentEntry.Name]; ok {
			_ = agent.SetUserQuestionsService(context.Background(), uqID)
		}
	} else {
		m.logger.Warn("user-questions service unavailable", "error", err)
	}

	// 按 DependsOn 对 provider 做拓扑排序；未能满足依赖的进入 PENDING
	sorted, pending := topoSortPlugins(providerEntries, declared)
	for _, e := range pending {
		m.markPendingLocked(e.Name, e.Type, "dependency not satisfied")
		m.pendingEntries[e.Name] = e // 记录待办条目，供动态注入补足依赖后提升
	}
	for _, entry := range sorted {
		if err := m.loadProviderDeclarativeLocked(entry); err != nil {
			m.transitionLocked(entry.Name, StateFailed, err.Error())
			return fmt.Errorf("failed to load plugin %s: %w", entry.Name, err)
		}
	}

	// provider 就绪后：确认 LLM、挂载聚合 Tool 服务并一次性注入 agent。
	// LLM 走多 provider 路由：agent 连接聚合 LLM 服务，primary 为声明的 provider。
	if agentEntry.DependsOn != nil {
		if _, ok := m.llms[agentEntry.DependsOn.LLM]; ok {
			hasLLM = true
		}
	}
	// 有工具插件加载或 agent 声明了工具依赖时，才提供聚合 Tool 服务
	if len(m.toolServiceIDs) > 0 || (agentEntry.DependsOn != nil && len(agentEntry.DependsOn.Tools) > 0) {
		if id, err := m.serveAggregateToolLocked(); err == nil {
			toolID = id
		} else {
			m.logger.Warn("aggregate tool service unavailable", "error", err)
		}
	}

	if hasLLM {
		agent, ok := m.agents[agentEntry.Name]
		if !ok {
			return fmt.Errorf("agent %s not loaded", agentEntry.Name)
		}
		if err := agent.RegisterServices(context.Background(), llmID, toolID); err != nil {
			return fmt.Errorf("failed to register agent services: %w", err)
		}
		m.agentServiceIDs[agentEntry.Name] = llmID // 记录注入的 LLM serviceID，热重载时沿用
		m.transitionLocked(agentEntry.Name, StateActive, "")
		m.logger.Info("agent activated declaratively", "name", agentEntry.Name, "llmID", llmID, "toolID", toolID)
	} else {
		m.markPendingLocked(agentEntry.Name, "agent", "dependent LLM not loaded")
		m.pendingEntries[agentEntry.Name] = *agentEntry // 记录待再激活的 agent 条目
		m.logger.Warn("agent deferred to pending (missing LLM dependency)", "name", agentEntry.Name)
	}

	m.logger.Info("all plugins loaded from config", "agent", agentEntry.Name)
	return nil
}

// loadProviderDeclarativeLocked 加载单个 provider 条目（需已持有 m.mu）。
// LLM 走原生进程内加载（loadLLMEntryLocked）后挂载为 broker 上的 gRPC 服务；
// Tool/Policy 走 loadPluginWithBroker（含类型/版本元数据校验）。
func (m *Manager) loadProviderDeclarativeLocked(entry PluginEntry) error {
	switch entry.Type {
	case "llm":
		if _, err := m.loadLLMEntryLocked(entry); err != nil {
			return err
		}
		if _, err := m.serveLLMProviderLocked(entry.Name); err != nil {
			return err
		}
	case "tool", "policy":
		if err := m.loadPluginWithBroker(entry, m.broker); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported provider type: %s", entry.Type)
	}
	return nil
}

// serveAggregateToolLocked 在 broker 上挂载「聚合 Tool 服务」并返回其 serviceID（需已持有 m.mu）。
// 该服务汇总所有工具插件注册表（ToolGRPCServer），供 agent 一次性注入，替代 Main 手工注册。
func (m *Manager) serveAggregateToolLocked() (uint32, error) {
	if m.broker == nil {
		return 0, fmt.Errorf("broker not available, cannot serve aggregate tool service")
	}
	serviceID := m.broker.NextId()
	go m.broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		proto.RegisterToolServiceServer(s, NewToolGRPCServer(m))
		return s
	})
	m.agentToolServiceID = serviceID
	return serviceID, nil
}

// serveAggregateToolOnBroker 在指定 broker 上挂载「聚合 Tool 服务」（NewToolGRPCServer，
// 请求经宿主 ExecuteTool 流水线转发到任意工具插件）；返回 serviceID。互通机制 4 中，
// 该服务须挂在本插件 client 的 broker 上（插件进程经自身 broker.Dial 访问），而不仅是
// agent broker——供 tool-lua-host 等「宿主内工具」经 dsc.tool.call 调用其他工具插件。
func (m *Manager) serveAggregateToolOnBroker(broker *goplugin.GRPCBroker) (uint32, error) {
	if broker == nil {
		return 0, fmt.Errorf("broker not available, cannot serve aggregate tool service")
	}
	serviceID := broker.NextId()
	go broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		proto.RegisterToolServiceServer(s, NewToolGRPCServer(m))
		return s
	})
	return serviceID, nil
}

// declaredPluginDeps 返回 entry 声明的、指向「整体已声明（启用）插件集」内插件的依赖名（去重）。
// 只有这些才参与拓扑排序；指向非插件名（如具体工具名 read_file）的引用由运行时解析，不算拓扑依赖。
func declaredPluginDeps(entry PluginEntry, declared map[string]bool) []string {
	var deps []string
	seen := make(map[string]bool)
	add := func(n string) {
		if n != "" && declared[n] && !seen[n] {
			seen[n] = true
			deps = append(deps, n)
		}
	}
	if entry.DependsOn != nil {
		add(entry.DependsOn.LLM)
		for _, t := range entry.DependsOn.Tools {
			add(t)
		}
	}
	return deps
}

// topoSortPlugins 依 DependsOn 对启用条目做稳定拓扑排序（Kahn），返回两个集合：
//   - sorted：依赖已满足、可立即加载的条目（依赖先于依赖者）；
//   - pending：依赖指向本批声明插件但未满足（如指向禁用/缺失的插件）的条目。
//
// 环形依赖已在调用前置的 CheckCircularDependencies 拦截，故此处的 pending 均为「缺依赖」。
func topoSortPlugins(entries []PluginEntry, declared map[string]bool) (sorted, pending []PluginEntry) {
	byName := make(map[string]PluginEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}

	// 每个节点的拓扑依赖（去重）
	depOf := make(map[string][]string, len(entries))
	for _, e := range entries {
		depOf[e.Name] = declaredPluginDeps(e, declared)
	}

	indeg := make(map[string]int, len(entries))
	dependents := make(map[string][]string)
	for n, ds := range depOf {
		indeg[n] = len(ds)
		for _, d := range ds {
			dependents[d] = append(dependents[d], n)
		}
	}

	// 就绪队列：入度为 0 的节点
	ready := make([]string, 0, len(entries))
	for n, d := range indeg {
		if d == 0 {
			ready = append(ready, n)
		}
	}

	head := 0
	for head < len(ready) {
		n := ready[head]
		head++
		// 依赖中途被判定未满足的节点不会入队；此处仅处理已入队节点
		if _, ok := byName[n]; !ok {
			continue
		}
		sorted = append(sorted, byName[n])
		for _, dep := range dependents[n] {
			indeg[dep]--
			if indeg[dep] == 0 {
				ready = append(ready, dep)
			}
		}
	}

	placed := make(map[string]bool, len(sorted))
	for _, e := range sorted {
		placed[e.Name] = true
	}
	for _, e := range entries {
		if !placed[e.Name] {
			pending = append(pending, e)
		}
	}
	return sorted, pending
}

// loadAgentAndGetBroker 加載 Agent 插件，返回 broker 和 Agent 實例（尚未設置依賴）
func (m *Manager) loadAgentAndGetBroker(entry PluginEntry) (*goplugin.GRPCBroker, Agent, error) {
	if _, exists := m.agents[entry.Name]; exists {
		m.unloadAgentLocked(entry.Name)
	}
	m.trackStateLocked(entry.Name, "agent")

	// 跨平台處理二進制路徑
	binaryPath := normalizeBinaryPath(entry.BinaryPath)

	// 校驗插件目錄命名規範
	dirName := getPluginDirectoryName(binaryPath)
	if err := validatePluginDirectoryName("agent", dirName); err != nil {
		return nil, nil, fmt.Errorf("invalid Agent plugin directory name '%s': %w", dirName, err)
	}

	cmd := exec.Command(binaryPath)
	if m.config.ExecDir != "" {
		cmd.Dir = m.config.ExecDir
	}
	cmd.Env = buildEnv(entry.Env)
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"agent": &AgentGRPCPlugin{},
		},
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, err.Error())
		return nil, nil, fmt.Errorf("failed to connect to agent plugin: %w", err)
	}
	m.transitionLocked(entry.Name, StateConnecting, "")

	grpcClient, ok := rpcClient.(*goplugin.GRPCClient)
	if !ok {
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, "agent plugin is not a gRPC client")
		return nil, nil, fmt.Errorf("agent plugin is not a gRPC client")
	}

	broker := grpcClient.Broker()

	// 獲取 Agent 實例（但不需要立即設置 LLM 服務 ID）
	raw, err := rpcClient.Dispense("agent")
	if err != nil {
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, err.Error())
		return nil, nil, fmt.Errorf("failed to dispense agent plugin: %w", err)
	}
	agent, ok := raw.(Agent)
	if !ok {
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, "plugin does not implement Agent interface")
		return nil, nil, fmt.Errorf("plugin does not implement Agent interface")
	}

	// 存儲客戶端和 agent（不設置 serviceID，稍後設置）
	m.clients[entry.Name] = client
	m.agents[entry.Name] = agent
	m.typeMap[entry.Name] = "agent"
	m.agentServiceIDs[entry.Name] = 0  // 占位
	m.agentEntries[entry.Name] = entry // 记录声明条目（含 DependsOn），供 PENDING 再激活时解析依赖
	// 注册对称清理 hook：卸载/热重载时先优雅关闭 agent（进程内清理），再终止进程
	a := agent
	m.addStopHookLocked(entry.Name, func() error {
		return a.Shutdown(context.Background(), false)
	})
	m.transitionLocked(entry.Name, StateReady, "")
	go m.monitorExit(entry.Name, client)

	m.logger.Info("agent plugin loaded for broker", "name", entry.Name)
	return broker, agent, nil
}

// loadPluginWithBroker 用於 LLM/Tool 插件加載，通過 broker 註冊服務
func (m *Manager) loadPluginWithBroker(entry PluginEntry, broker *goplugin.GRPCBroker) error {
	// 跨平台處理二進制路徑
	binaryPath := entry.BinaryPath
	if binaryPath == "" {
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}
		// 根據插件名稱生成二進制路徑：./plugins/<name>/<name>.exe
		binaryPath = fmt.Sprintf("./plugins/%s/%s%s", entry.Name, entry.Name, ext)
	}
	binaryPath = normalizeBinaryPath(binaryPath)

	// 校驗插件目錄命名規範
	dirName := getPluginDirectoryName(binaryPath)
	if err := validatePluginDirectoryName(entry.Type, dirName); err != nil {
		return fmt.Errorf("invalid plugin directory name '%s' for type '%s': %w", dirName, entry.Type, err)
	}
	m.trackStateLocked(entry.Name, entry.Type)

	// 創建客戶端
	cmd := exec.Command(binaryPath)
	if m.config.ExecDir != "" {
		cmd.Dir = m.config.ExecDir
	}
	cmd.Env = m.pluginEnv(entry)
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"dsc_plugin": &DSCPluginGRPC{},
		},
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, err.Error())
		return fmt.Errorf("failed to connect to plugin: %w", err)
	}
	m.transitionLocked(entry.Name, StateConnecting, "")

	grpcClient, ok := rpcClient.(*goplugin.GRPCClient)
	if !ok {
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, "plugin is not a gRPC client")
		return fmt.Errorf("plugin is not a gRPC client")
	}

	// 獲取元數據
	info, err := GetPluginInfo(grpcClient.Conn)
	if err != nil {
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, err.Error())
		return fmt.Errorf("failed to get plugin info: %w", err)
	}

	// 版本兼容性檢查
	cons, err := version.NewConstraint(">= 1.0, < 2.0")
	if err != nil {
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, err.Error())
		return fmt.Errorf("failed to create version constraint: %w", err)
	}

	v, err := version.NewVersion(info.ApiVersion)
	if err != nil {
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, err.Error())
		return fmt.Errorf("invalid API version %s: %w", info.ApiVersion, err)
	}

	if !cons.Check(v) {
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, fmt.Sprintf("unsupported API version %s", info.ApiVersion))
		return fmt.Errorf("unsupported API version %s, expected >=1.0 <2.0", info.ApiVersion)
	}

	if info.Type != entry.Type {
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, fmt.Sprintf("plugin type mismatch: expected %s, got %s", entry.Type, info.Type))
		return fmt.Errorf("plugin type mismatch: expected %s, got %s", entry.Type, info.Type)
	}
	m.transitionLocked(entry.Name, StateReady, "")

	switch info.Type {
	case "llm":
		llmClient := proto.NewLLMServiceClient(grpcClient.Conn)
		proxy := &llmProxyServer{client: llmClient}
		serviceID := broker.NextId()
		go broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
			s := grpc.NewServer(opts...)
			proto.RegisterLLMServiceServer(s, proxy)
			return s
		})
		m.llmServiceIDs[entry.Name] = serviceID
		m.clients[entry.Name] = client
		m.typeMap[entry.Name] = "llm"
		m.pluginMetadata[entry.Name] = info
		m.transitionLocked(entry.Name, StateActive, "")
		go m.monitorExit(entry.Name, client)
		m.logger.Info("LLM service registered", "name", entry.Name, "serviceID", serviceID)

	case "tool":
		toolClient := proto.NewToolServiceClient(grpcClient.Conn)
		serviceID := broker.NextId()

		proxy := &toolProxyServer{client: toolClient}
		go broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
			s := grpc.NewServer(opts...)
			proto.RegisterToolServiceServer(s, proxy)
			return s
		})
		m.toolServiceIDs[entry.Name] = serviceID
		m.toolClients[entry.Name] = toolClient
		// 互通机制 3：插件钩子回调客户端（插件未注册 PluginHookService 时
		// 调用返回 UNIMPLEMENTED，宿主容错跳过）
		m.toolHookClients[entry.Name] = proto.NewPluginHookServiceClient(grpcClient.Conn)
		m.toolHookOrder = append(m.toolHookOrder, entry.Name)
		// 互通机制 1/2/4：聚合 LLM、聚合 Tool 与插件通知服务须挂在本插件 client 的
		// broker 上（插件进程经自身 broker.Dial 访问）；serviceID 经 SetInterconnect
		// 传给插件进程（旧插件未实现则 UNIMPLEMENTED 容错跳过）。
		if pBroker := grpcClient.Broker(); pBroker != nil {
			llmID := uint32(0)
			if m.agentLLMServiceID != 0 {
				if id, err := m.serveAggregateLLMOnBroker(pBroker, m.agentLLMName); err == nil {
					llmID = id
				}
			}
			toolID := uint32(0)
			if id, err := m.serveAggregateToolOnBroker(pBroker); err == nil {
				toolID = id
			}
			notifyID := uint32(0)
			if id, err := m.servePluginNotifyOnBroker(pBroker); err == nil {
				notifyID = id
			}
			if llmID != 0 || toolID != 0 || notifyID != 0 {
				ictx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				_, err := toolClient.SetInterconnect(ictx, &proto.InterconnectRequest{
					LlmServiceId: llmID, NotifyServiceId: notifyID, ToolServiceId: toolID,
				})
				cancel()
				if err != nil {
					m.logger.Warn("plugin does not support interconnect", "plugin", entry.Name, "error", err)
				}
			}
		}

		// 工具清单在 SetInterconnect 之后拉取：宿主内工具（如 tool-lua-host）的脚本
		// 在握手时加载，必须先握手再列工具，否则其注册的工具会缺失。
		listCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		listResp, err := toolClient.ListTools(listCtx, &proto.ListToolsRequest{})
		cancel()
		if err != nil {
			client.Kill()
			m.transitionLocked(entry.Name, StateFailed, err.Error())
			return fmt.Errorf("failed to list tools after retries: %w", err)
		}
		var toolNames []string
		for _, t := range listResp.Tools {
			remote := &RemoteTool{
				name:        t.Name,
				description: t.Description,
				schema:      json.RawMessage(t.ParametersJson),
				client:      toolClient,
			}
			if err := m.toolRegistry.Register(remote); err != nil {
				m.logger.Warn("failed to register tool", "tool", t.Name, "error", err)
				continue
			}
			toolNames = append(toolNames, t.Name)
			m.toolNameToServiceID[t.Name] = serviceID
		}
		m.pluginToolNames[entry.Name] = toolNames
		m.clients[entry.Name] = client
		m.typeMap[entry.Name] = "tool"
		m.pluginMetadata[entry.Name] = info
		// 注册对称清理 hook：卸载/热重载时先撤销该插件的全部工具，再终止进程
		m.addStopHookLocked(entry.Name, func() error {
			m.unregisterPluginToolsLocked(entry.Name)
			return nil
		})
		m.transitionLocked(entry.Name, StateActive, "")
		go m.monitorExit(entry.Name, client)
		m.logger.Info("Tool service registered", "name", entry.Name, "serviceID", serviceID)

	case "policy":
		m.clients[entry.Name] = client
		m.typeMap[entry.Name] = "policy"
		m.pluginMetadata[entry.Name] = info
		// 桥接：策略观测服务挂在插件主 gRPC server 上，经主连接取 client
		// 并注册为工具流水线监听器（替代旁路）
		pc := proto.NewFsObservationPolicyServiceClient(grpcClient.Conn)
		m.policyClients[entry.Name] = pc
		m.policyOff[entry.Name] = m.bridgePolicyToPipeline(entry.Name, pc)
		m.transitionLocked(entry.Name, StateActive, "")
		go m.monitorExit(entry.Name, client)
		m.logger.Info("Policy plugin loaded and bridged to tool pipeline", "name", entry.Name)

	default:
		client.Kill()
		m.transitionLocked(entry.Name, StateFailed, fmt.Sprintf("unsupported plugin type: %s", info.Type))
		return fmt.Errorf("unsupported plugin type: %s", info.Type)
	}
	return nil
}

// buildEnv 合併宿主環境與插件自定義環境變量，插件變量優先
func buildEnv(custom map[string]string) []string {
	envMap := make(map[string]string)
	for _, kv := range os.Environ() {
		if i := strings.Index(kv, "="); i > 0 {
			envMap[kv[:i]] = kv[i+1:]
		}
	}
	for k, v := range custom {
		envMap[k] = v
	}
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, k+"="+v)
	}
	return env
}

// pluginEnv 计算插件进程 env：宿主环境 + 插件自定义 env。
// 互通服务 ID 不再经 env 注入（握手时序问题）：改为宿主加载工具插件后经
// ToolService.SetInterconnect 把挂载在本插件 client broker 上的服务 ID 传入
// 插件进程（互通机制 1/2）。
func (m *Manager) pluginEnv(entry PluginEntry) []string {
	return buildEnv(entry.Env)
}

// LoadToolsAndPoliciesFromConfig 從配置加載 tool 和 policy 插件
func (m *Manager) LoadToolsAndPoliciesFromConfig(cfg *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range cfg.Plugins {
		if !entry.Enabled {
			continue
		}
		if entry.Type == "tool" || entry.Type == "policy" {
			if err := m.loadPluginWithBroker(entry, m.broker); err != nil {
				return fmt.Errorf("failed to load plugin %s: %w", entry.Name, err)
			}
		}
	}
	return nil
}

// SwitchMode 實時切換工作模式（minimal / standard）
func (m *Manager) SwitchMode(mode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	presetPath := fmt.Sprintf("config/presets/%s.yaml", mode)
	// 使用基於 ExecDir 或可執行文件所在目錄的絕對路徑
	if m.config.ExecDir != "" {
		presetPath = filepath.Join(m.config.ExecDir, "config", "presets", fmt.Sprintf("%s.yaml", mode))
	} else {
		// 嘗試獲取可執行文件所在目錄
		if execPath, err := os.Executable(); err == nil {
			execDir := filepath.Dir(execPath)
			presetPath = filepath.Join(execDir, "config", "presets", fmt.Sprintf("%s.yaml", mode))
		}
	}

	data, err := os.ReadFile(presetPath)
	if err != nil {
		return fmt.Errorf("failed to read preset config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to unmarshal preset config: %w", err)
	}

	// 獲取當前已加載的 tool/policy 插件列表
	currentTools := make(map[string]bool)
	for name, typ := range m.typeMap {
		if typ == "tool" || typ == "policy" {
			currentTools[name] = true
		}
	}

	// 獲取目標模式的 tool/policy 插件列表
	targetTools := make(map[string]bool)
	for _, entry := range cfg.Plugins {
		if (entry.Type == "tool" || entry.Type == "policy") && entry.Enabled {
			targetTools[entry.Name] = true
		}
	}

	// 卸載不再需要的插件
	for name := range currentTools {
		if !targetTools[name] {
			// 卸載 tool/plugin
			if client, exists := m.clients[name]; exists {
				m.transitionLocked(name, StateUnloading, "")
				delete(m.clients, name) // 先摘除，令退出监控忽略
				client.Kill()
				m.markDisposedLocked(name)
			}
			delete(m.typeMap, name)
			delete(m.pluginMetadata, name)

			// 從工具註冊表中移除該插件註冊的所有工具，
			// 否則 ListTools 仍會返回已下線插件的工具，模型會誤報多餘的工具
			if toolNames, exists := m.pluginToolNames[name]; exists {
				for _, tname := range toolNames {
					delete(m.toolNameToServiceID, tname)
					m.toolRegistry.Unregister(tname)
				}
			}
			delete(m.toolServiceIDs, name)
			delete(m.pluginToolNames, name)
			delete(m.toolClients, name)

			m.logger.Info("plugin unloaded during mode switch", "name", name, "mode", mode)
		}
	}

	// 加載新的插件
	for _, entry := range cfg.Plugins {
		if (entry.Type == "tool" || entry.Type == "policy") && entry.Enabled {
			if !currentTools[entry.Name] {
				// 注入当前模式（DSC_MODE）：tool-lua-host 据此限制插件创造仅在创造模式下允许
				if entry.Env == nil {
					entry.Env = map[string]string{}
				}
				entry.Env["DSC_MODE"] = mode
				if err := m.loadPluginWithBroker(entry, m.broker); err != nil {
					return fmt.Errorf("failed to load plugin %s: %w", entry.Name, err)
				}
				m.logger.Info("plugin loaded during mode switch", "name", entry.Name, "mode", mode)
			}
		}
	}

	return nil
}
