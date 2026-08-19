package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dsc/proto"
	"dsc/proto/metadata"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/hashicorp/go-version"
	"gopkg.in/yaml.v3"
	"google.golang.org/grpc"
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
	broker              *goplugin.GRPCBroker // 統一的 broker，由 Agent 提供
	mainAgentName       string               // 主 Agent 名稱
	llmServiceIDs       map[string]uint32    // llm name -> serviceID
	toolServiceIDs      map[string]uint32    // tool plugin name -> serviceID
	pluginToolNames     map[string][]string  // tool plugin name -> list of tool names it provides
	toolNameToServiceID map[string]uint32    // tool name -> serviceID
}

type ManagerConfig struct {
	PluginDir    string
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
		pluginLogger:        pluginLogger,
	}
	// 註冊內置工具（現已遷移至獨立插件 tool-str-replace-editor）
	// 後續可註冊更多工具
	return m
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

	// 創建插件客戶端
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"dsc_plugin": &DSCPluginGRPC{},
		},
		Cmd:              exec.Command(binaryPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	// 建立 RPC 連接
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to connect to plugin: %w", err)
	}

	// 獲取插件實例
	raw, err := rpcClient.Dispense("dsc_plugin")
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to dispense plugin: %w", err)
	}

	impl, ok := raw.(DSCPlugin)
	if !ok {
		client.Kill()
		return fmt.Errorf("plugin does not implement DSCPlugin interface")
	}

	m.clients[name] = client
	m.plugins[name] = impl
	m.typeMap[name] = "dsc"

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
		client.Kill() // 殺死插件子進程
		delete(m.clients, name)
		delete(m.plugins, name)
		delete(m.typeMap, name)
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

	// 構建命令，加入 -llm-service-id 參數
	cmdArgs := []string{"-llm-service-id", strconv.FormatUint(uint64(serviceID), 10)}
	cmd := exec.Command(binaryPath, cmdArgs...)

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
		return fmt.Errorf("failed to connect to agent plugin: %w", err)
	}

	// 獲取插件實例
	raw, err := rpcClient.Dispense("agent")
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to dispense agent plugin: %w", err)
	}

	impl, ok := raw.(Agent)
	if !ok {
		client.Kill()
		return fmt.Errorf("plugin does not implement Agent interface")
	}

	m.clients[name] = client
	m.agents[name] = impl
	m.typeMap[name] = "agent"

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
		client.Kill() // 殺死插件子進程
		delete(m.clients, name)
		delete(m.agents, name)
		delete(m.typeMap, name)
		delete(m.agentServiceIDs, name)
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

	// 如果已加載，先卸載
	if _, exists := m.agents[name]; exists {
		m.unloadAgentLocked(name)
	}

	// 創建插件客戶端（先不傳遞 serviceID，稍後通過 broker 生成）
	cmd := exec.Command(binaryPath)
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
		return nil, 0, fmt.Errorf("failed to connect to agent plugin: %w", err)
	}

	grpcClient, ok := rpcClient.(*goplugin.GRPCClient)
	if !ok {
		client.Kill()
		return nil, 0, fmt.Errorf("agent plugin is not a gRPC client")
	}

	broker := grpcClient.Broker()
	m.broker = broker // 供後續 LoadToolsAndPoliciesFromConfig 等使用
	serviceID := broker.NextId()

	// 獲取 Agent 實例（用於調用 RPC）
	raw, err := rpcClient.Dispense("agent")
	if err != nil {
		client.Kill()
		return nil, 0, fmt.Errorf("failed to dispense agent plugin: %w", err)
	}

	impl, ok := raw.(Agent)
	if !ok {
		client.Kill()
		return nil, 0, fmt.Errorf("plugin does not implement Agent interface")
	}

	// 透過 RPC 設置 serviceID
	agentClient := proto.NewAgentServiceClient(grpcClient.Conn)
	_, err = agentClient.SetLLMServiceID(context.Background(), &proto.SetLLMServiceIDRequest{ServiceId: serviceID})
	if err != nil {
		client.Kill()
		return nil, 0, fmt.Errorf("failed to set service ID on agent: %w", err)
	}

	m.clients[name] = client
	m.agents[name] = impl
	m.typeMap[name] = "agent"
	m.agentServiceIDs[name] = serviceID

	m.logger.Info("agent plugin loaded", "name", name, "serviceID", serviceID)
	return broker, serviceID, nil
}

// LoadLLM 加載 LLM 插件；env 為傳遞給插件子進程的自定義環境變量（與宿主環境合併，插件值優先）
func (m *Manager) LoadLLM(name string, binaryPath string, env map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果已加載，先卸載
	if _, exists := m.llms[name]; exists {
		m.unloadLLMLocked(name)
	}

	// 創建插件客戶端
	cmd := exec.Command(binaryPath)
	if len(env) > 0 {
		cmd.Env = buildEnv(env)
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
		return fmt.Errorf("failed to connect to LLM plugin: %w", err)
	}

	// 獲取插件實例
	raw, err := rpcClient.Dispense("llm")
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to dispense LLM plugin: %w", err)
	}

	impl, ok := raw.(LLMProvider)
	if !ok {
		client.Kill()
		return fmt.Errorf("plugin does not implement LLMProvider interface")
	}

	m.clients[name] = client
	m.llms[name] = impl
	m.typeMap[name] = "llm"

	m.logger.Info("llm plugin loaded", "name", name)
	return nil
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
		client.Kill() // 殺死插件子進程
		delete(m.clients, name)
		delete(m.llms, name)
		delete(m.typeMap, name)
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
	newClient := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"dsc_plugin": &DSCPluginGRPC{},
		},
		Cmd:              exec.Command(newBinaryPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	rpcClient, err := newClient.Client()
	if err != nil {
		newClient.Kill()
		return fmt.Errorf("new plugin connection failed: %w", err)
	}

	raw, err := rpcClient.Dispense("dsc_plugin")
	if err != nil {
		newClient.Kill()
		return fmt.Errorf("dispense new plugin failed: %w", err)
	}

	newImpl, ok := raw.(DSCPlugin)
	if !ok {
		newClient.Kill()
		return fmt.Errorf("new plugin does not implement DSCPlugin interface")
	}

	// 2. 卸載舊 client（若存在）
	if oldClient, exists := m.clients[name]; exists {
		oldClient.Kill()
	}
	// 3. 更新映射
	m.clients[name] = newClient
	m.plugins[name] = newImpl
	m.typeMap[name] = "dsc"

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
	newClient := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"agent": &AgentGRPCPlugin{},
		},
		Cmd:              exec.Command(newBinaryPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	rpcClient, err := newClient.Client()
	if err != nil {
		newClient.Kill()
		return fmt.Errorf("new agent plugin connection failed: %w", err)
	}

	grpcClient, ok := rpcClient.(*goplugin.GRPCClient)
	if !ok {
		newClient.Kill()
		return fmt.Errorf("new agent plugin is not a gRPC client")
	}

	// 透過 RPC 設置 serviceID
	agentClient := proto.NewAgentServiceClient(grpcClient.Conn)
	_, err = agentClient.SetLLMServiceID(context.Background(), &proto.SetLLMServiceIDRequest{ServiceId: oldServiceID})
	if err != nil {
		newClient.Kill()
		return fmt.Errorf("failed to set service ID on agent: %w", err)
	}

	raw, err := rpcClient.Dispense("agent")
	if err != nil {
		newClient.Kill()
		return fmt.Errorf("dispense new agent plugin failed: %w", err)
	}

	newImpl, ok := raw.(Agent)
	if !ok {
		newClient.Kill()
		return fmt.Errorf("new agent plugin does not implement Agent interface")
	}

	// 2. 卸載舊 client（若存在），先嘗試優雅關閉
	if oldAgent, exists := m.agents[name]; exists {
		// 嘗試優雅關閉，不強制
		err := oldAgent.Shutdown(context.Background(), false)
		if err != nil {
			m.logger.Warn("agent shutdown failed, falling back to kill", "name", name, "error", err)
		}
	}
	if oldClient, exists := m.clients[name]; exists {
		oldClient.Kill()
	}
	// 3. 更新映射
	m.clients[name] = newClient
	m.agents[name] = newImpl
	m.typeMap[name] = "agent"

	m.logger.Info("agent plugin hot-reloaded", "name", name, "serviceID", oldServiceID)
	return nil
}

// hotReloadLLMLocked 內部重載 LLM（需已持有鎖）
func (m *Manager) hotReloadLLMLocked(name, newBinaryPath string) error {
	// 1. 建立新 client 並驗證
	newClient := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"llm": &LLMGRPCPlugin{},
		},
		Cmd:              exec.Command(newBinaryPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           m.pluginLogger,
	})

	rpcClient, err := newClient.Client()
	if err != nil {
		newClient.Kill()
		return fmt.Errorf("new LLM plugin connection failed: %w", err)
	}

	raw, err := rpcClient.Dispense("llm")
	if err != nil {
		newClient.Kill()
		return fmt.Errorf("dispense new LLM plugin failed: %w", err)
	}

	newImpl, ok := raw.(LLMProvider)
	if !ok {
		newClient.Kill()
		return fmt.Errorf("new LLM plugin does not implement LLMProvider interface")
	}

	// 2. 卸載舊 client（若存在）
	if oldClient, exists := m.clients[name]; exists {
		oldClient.Kill()
	}
	// 3. 更新映射
	m.clients[name] = newClient
	m.llms[name] = newImpl
	m.typeMap[name] = "llm"

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
	for name, client := range m.clients {
		client.Kill()
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

// ExecuteTool 執行工具（供 RPC 調用）
func (m *Manager) ExecuteTool(ctx context.Context, toolName string, argsJSON json.RawMessage) (string, error) {
	tool, ok := m.toolRegistry.Get(toolName)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", toolName)
	}
	return tool.Execute(ctx, argsJSON)
}

// UnloadPlugin 卸載插件
func (m *Manager) UnloadPlugin(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, exists := m.clients[name]
	if !exists {
		return fmt.Errorf("plugin '%s' not found", name)
	}

	// 從註冊表中移除工具
	if m.typeMap[name] == "tool" {
		// 獲取該工具插件註冊的工具名稱列表
		toolNames, hasToolNames := m.pluginToolNames[name]
		if hasToolNames {
			for _, toolName := range toolNames {
				m.toolRegistry.Unregister(toolName)
				delete(m.toolNameToServiceID, toolName)
				m.logger.Info("unregistered tool", "tool", toolName, "plugin", name)
			}
		}
		// 刪除相關記錄
		delete(m.toolServiceIDs, name)
		delete(m.pluginToolNames, name)
	} else if m.typeMap[name] == "llm" {
		delete(m.llmServiceIDs, name)
	}

	client.Kill() // 殺死插件子進程
	delete(m.clients, name)
	delete(m.plugins, name)
	delete(m.agents, name)
	delete(m.llms, name)
	delete(m.typeMap, name)
	delete(m.pluginMetadata, name)

	m.logger.Info("plugin unloaded", "name", name)
	return nil
}

// PluginInfoSummary 插件信息摘要
type PluginInfoSummary struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
}

// ListPlugins 返回所有已加載插件的列表
func (m *Manager) ListPlugins() []PluginInfoSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var plugins []PluginInfoSummary
	for name, info := range m.pluginMetadata {
		plugins = append(plugins, PluginInfoSummary{
			Name:    name,
			Type:    info.Type,
			Version: info.Version,
			Enabled: true,
		})
	}
	for name, typ := range m.typeMap {
		// 如果已經在 pluginMetadata 中，則跳過
		if _, exists := m.pluginMetadata[name]; exists {
			continue
		}
		plugins = append(plugins, PluginInfoSummary{
			Name:    name,
			Type:    typ,
			Version: "unknown",
			Enabled: true,
		})
	}
	return plugins
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

// LoadPlugin 動態加載插件（供 Admin API 使用）
func (m *Manager) LoadPlugin(entry PluginEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果是 agent 插件
	if entry.Type == "agent" {
		_, _, err := m.loadAgentAndGetBroker(entry)
		return err
	}

	// 對於 llm/tool 插件，需要 broker
	if m.broker == nil {
		return fmt.Errorf("broker not available, cannot load plugin type %s", entry.Type)
	}

	return m.loadPluginWithBroker(entry, m.broker)
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

// LoadFromConfig 從配置加載所有插件（重新設計版：先加載 Agent 獲取 Broker，再加載 LLM/Tool）
func (m *Manager) LoadFromConfig(cfg *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 第一步：找到 Agent 插件（假設第一個類型為 agent 的）
	var agentEntry *PluginEntry
	var otherEntries []PluginEntry
	for _, entry := range cfg.Plugins {
		if !entry.Enabled {
			continue
		}
		if entry.Type == "agent" {
			if agentEntry == nil {
				agentEntry = &entry
			} else {
				m.logger.Warn("multiple agent plugins found, using first one", "first", agentEntry.Name, "ignored", entry.Name)
			}
		} else {
			otherEntries = append(otherEntries, entry)
		}
	}

	if agentEntry == nil {
		return fmt.Errorf("no agent plugin found in config")
	}

	// 第二步：加載 Agent 並獲取 Broker
	broker, agent, err := m.loadAgentAndGetBroker(*agentEntry)
	if err != nil {
		return fmt.Errorf("failed to load agent: %w", err)
	}
	m.broker = broker
	m.mainAgentName = agentEntry.Name

	// 第三步：加載其他插件（LLM、Tool）
	for _, entry := range otherEntries {
		if err := m.loadPluginWithBroker(entry, broker); err != nil {
			return fmt.Errorf("failed to load plugin %s: %w", entry.Name, err)
		}
	}

	// 第四步：為 Agent 設置 LLM 和 Tool 服務 ID
	if agentEntry.DependsOn != nil {
		var llmID, toolID uint32
		hasLLM := false
		hasTool := false

		if agentEntry.DependsOn.LLM != "" {
			if id, ok := m.llmServiceIDs[agentEntry.DependsOn.LLM]; ok {
				llmID = id
				hasLLM = true
			} else {
				return fmt.Errorf("dependent LLM '%s' not loaded", agentEntry.DependsOn.LLM)
			}
		}

		if len(agentEntry.DependsOn.Tools) > 0 {
			toolName := agentEntry.DependsOn.Tools[0]
			// 依賴值可為工具名（如 read_file），也可為提供該工具的插件名（如 filesystem）
			if id, ok := m.toolNameToServiceID[toolName]; ok {
				toolID = id
				hasTool = true
			} else if id, ok := m.toolServiceIDs[toolName]; ok {
				toolID = id
				hasTool = true
			} else {
				return fmt.Errorf("dependent Tool '%s' not loaded", toolName)
			}
		}

		// 調用 Agent 的 RPC 設置 ID
		if err := agent.SetLLMServiceID(context.Background(), llmID); err != nil && hasLLM {
			return fmt.Errorf("failed to set LLM service ID: %w", err)
		}
		if err := agent.SetToolServiceID(context.Background(), toolID); err != nil && hasTool {
			return fmt.Errorf("failed to set Tool service ID: %w", err)
		}
		m.logger.Info("agent dependencies set", "llmID", llmID, "toolID", toolID)
	}

	m.logger.Info("all plugins loaded from config", "agent", agentEntry.Name)
	return nil
}

// loadAgentAndGetBroker 加載 Agent 插件，返回 broker 和 Agent 實例（尚未設置依賴）
func (m *Manager) loadAgentAndGetBroker(entry PluginEntry) (*goplugin.GRPCBroker, Agent, error) {
	if _, exists := m.agents[entry.Name]; exists {
		m.unloadAgentLocked(entry.Name)
	}

	// 跨平台處理二進制路徑
	binaryPath := normalizeBinaryPath(entry.BinaryPath)
	cmd := exec.Command(binaryPath)
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
		return nil, nil, fmt.Errorf("failed to connect to agent plugin: %w", err)
	}

	grpcClient, ok := rpcClient.(*goplugin.GRPCClient)
	if !ok {
		client.Kill()
		return nil, nil, fmt.Errorf("agent plugin is not a gRPC client")
	}

	broker := grpcClient.Broker()

	// 獲取 Agent 實例（但不需要立即設置 LLM 服務 ID）
	raw, err := rpcClient.Dispense("agent")
	if err != nil {
		client.Kill()
		return nil, nil, fmt.Errorf("failed to dispense agent plugin: %w", err)
	}
	agent, ok := raw.(Agent)
	if !ok {
		client.Kill()
		return nil, nil, fmt.Errorf("plugin does not implement Agent interface")
	}

	// 存儲客戶端和 agent（不設置 serviceID，稍後設置）
	m.clients[entry.Name] = client
	m.agents[entry.Name] = agent
	m.typeMap[entry.Name] = "agent"
	m.agentServiceIDs[entry.Name] = 0 // 占位

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

	// 創建客戶端
	cmd := exec.Command(binaryPath)
	cmd.Env = buildEnv(entry.Env)
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
		return fmt.Errorf("failed to connect to plugin: %w", err)
	}

	grpcClient, ok := rpcClient.(*goplugin.GRPCClient)
	if !ok {
		client.Kill()
		return fmt.Errorf("plugin is not a gRPC client")
	}

	// 獲取元數據
	info, err := GetPluginInfo(grpcClient.Conn)
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to get plugin info: %w", err)
	}

	// 版本兼容性檢查
	cons, err := version.NewConstraint(">= 1.0, < 2.0")
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to create version constraint: %w", err)
	}

	v, err := version.NewVersion(info.ApiVersion)
	if err != nil {
		client.Kill()
		return fmt.Errorf("invalid API version %s: %w", info.ApiVersion, err)
	}

	if !cons.Check(v) {
		client.Kill()
		return fmt.Errorf("unsupported API version %s, expected >=1.0 <2.0", info.ApiVersion)
	}

	if info.Type != entry.Type {
		client.Kill()
		return fmt.Errorf("plugin type mismatch: expected %s, got %s", entry.Type, info.Type)
	}

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
		m.logger.Info("LLM service registered", "name", entry.Name, "serviceID", serviceID)

	case "tool":
		toolClient := proto.NewToolServiceClient(grpcClient.Conn)
		serviceID := broker.NextId()

		// 为 gRPC 调用创建带超时的 context
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var listResp *proto.ListToolsResponse
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			listResp, err = toolClient.ListTools(ctx, &proto.ListToolsRequest{})
			if err == nil {
				break
			}
			time.Sleep(time.Duration(1<<attempt) * 100 * time.Millisecond)
		}
		if err != nil {
			client.Kill()
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
		proxy := &toolProxyServer{client: toolClient}
		go broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
			s := grpc.NewServer(opts...)
			proto.RegisterToolServiceServer(s, proxy)
			return s
		})
		m.toolServiceIDs[entry.Name] = serviceID
		m.pluginToolNames[entry.Name] = toolNames
		m.clients[entry.Name] = client
		m.typeMap[entry.Name] = "tool"
		m.pluginMetadata[entry.Name] = info
		m.logger.Info("Tool service registered", "name", entry.Name, "serviceID", serviceID)

	case "policy":
		serviceID := broker.NextId()
		m.clients[entry.Name] = client
		m.typeMap[entry.Name] = "policy"
		m.pluginMetadata[entry.Name] = info
		m.logger.Info("Policy plugin loaded", "name", entry.Name, "serviceID", serviceID)

	default:
		client.Kill()
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
				client.Kill()
			}
			delete(m.clients, name)
			delete(m.typeMap, name)
			delete(m.pluginMetadata, name)

			// 清理 tool 相關的映射
			if _, ok := m.toolServiceIDs[name]; ok {
				// 移除 toolNameToServiceID 中的條目
				if toolNames, exists := m.pluginToolNames[name]; exists {
					for _, tname := range toolNames {
						delete(m.toolNameToServiceID, tname)
					}
				}
				delete(m.toolServiceIDs, name)
				delete(m.pluginToolNames, name)
			}

			m.logger.Info("plugin unloaded during mode switch", "name", name, "mode", mode)
		}
	}

	// 加載新的插件
	for _, entry := range cfg.Plugins {
		if (entry.Type == "tool" || entry.Type == "policy") && entry.Enabled {
			if !currentTools[entry.Name] {
				if err := m.loadPluginWithBroker(entry, m.broker); err != nil {
					return fmt.Errorf("failed to load plugin %s: %w", entry.Name, err)
				}
				m.logger.Info("plugin loaded during mode switch", "name", entry.Name, "mode", mode)
			}
		}
	}

	return nil
}
