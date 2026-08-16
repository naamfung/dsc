package plugin

import (
	"fmt"
	"os/exec"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
)

// Manager 插件管理器
type Manager struct {
	mu      sync.RWMutex
	clients map[string]*plugin.Client // 插件名 -> 客戶端
	plugins map[string]DSCPlugin      // 插件名 -> DSC業務接口
	agents  map[string]Agent          // 插件名 -> Agent接口
	llms    map[string]LLMProvider    // 插件名 -> LLMProvider接口
	config  *ManagerConfig
}

type ManagerConfig struct {
	PluginDir string
	Handshake plugin.HandshakeConfig
}

func NewManager(cfg *ManagerConfig) *Manager {
	return &Manager{
		clients: make(map[string]*plugin.Client),
		plugins: make(map[string]DSCPlugin),
		agents:  make(map[string]Agent),
		llms:    make(map[string]LLMProvider),
		config:  cfg,
	}
}

// Load 加載一個插件（啟動子進程）
func (m *Manager) Load(name string, binaryPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果已加載，先卸載
	if _, exists := m.plugins[name]; exists {
		m.unloadLocked(name)
	}

	// 創建插件客戶端
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]plugin.Plugin{
			"dsc_plugin": &DSCPluginGRPC{},
		},
		Cmd:              exec.Command(binaryPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.New(&hclog.LoggerOptions{Name: "plugin"}),
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

	fmt.Printf("[Manager] Plugin '%s' loaded successfully\n", name)
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
		fmt.Printf("[Manager] Plugin '%s' unloaded\n", name)
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
func (m *Manager) LoadAgent(name string, binaryPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果已加載，先卸載
	if _, exists := m.agents[name]; exists {
		m.unloadAgentLocked(name)
	}

	// 創建插件客戶端
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]plugin.Plugin{
			"agent": &AgentGRPCPlugin{},
		},
		Cmd:              exec.Command(binaryPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.New(&hclog.LoggerOptions{Name: "plugin"}),
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

	fmt.Printf("[Manager] Agent plugin '%s' loaded successfully\n", name)
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
		fmt.Printf("[Manager] Agent plugin '%s' unloaded\n", name)
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

// LoadLLM 加載 LLM 插件
func (m *Manager) LoadLLM(name string, binaryPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果已加載，先卸載
	if _, exists := m.llms[name]; exists {
		m.unloadLLMLocked(name)
	}

	// 創建插件客戶端
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]plugin.Plugin{
			"llm": &LLMGRPCPlugin{},
		},
		Cmd:              exec.Command(binaryPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.New(&hclog.LoggerOptions{Name: "llm-plugin"}),
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

	fmt.Printf("[Manager] LLM plugin '%s' loaded successfully\n", name)
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
		fmt.Printf("[Manager] LLM plugin '%s' unloaded\n", name)
		return nil
	}
	return fmt.Errorf("LLM plugin '%s' not found", name)
}

// HotReload 熱重載：卸載舊版本，加載新版本
// 支持 DSCPlugin、Agent、LLM 三種類型的插件，根據當前 name 對應的類型自動判斷
func (m *Manager) HotReload(name string, newBinaryPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 判斷類型並調用對應的重載邏輯
	if _, ok := m.plugins[name]; ok {
		return m.hotReloadPluginLocked(name, newBinaryPath)
	}
	if _, ok := m.agents[name]; ok {
		return m.hotReloadAgentLocked(name, newBinaryPath)
	}
	if _, ok := m.llms[name]; ok {
		return m.hotReloadLLMLocked(name, newBinaryPath)
	}
	return fmt.Errorf("plugin '%s' not found (no registered type)", name)
}

// hotReloadPluginLocked 內部重載 DSCPlugin（需已持有鎖）
func (m *Manager) hotReloadPluginLocked(name, newBinaryPath string) error {
	// 1. 卸載舊的
	if err := m.unloadLocked(name); err != nil {
		// 忽略未找到錯誤，繼續加載
	}

	// 2. 加載新的（複製 Load 中的邏輯）
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]plugin.Plugin{
			"dsc_plugin": &DSCPluginGRPC{},
		},
		Cmd:              exec.Command(newBinaryPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.New(&hclog.LoggerOptions{Name: "plugin"}),
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to connect to plugin: %w", err)
	}

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

	fmt.Printf("[Manager] Plugin '%s' hot-reloaded\n", name)
	return nil
}

// hotReloadAgentLocked 內部重載 Agent（需已持有鎖）
func (m *Manager) hotReloadAgentLocked(name, newBinaryPath string) error {
	if err := m.unloadAgentLocked(name); err != nil {
		// 忽略未找到錯誤
	}

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]plugin.Plugin{
			"agent": &AgentGRPCPlugin{},
		},
		Cmd:              exec.Command(newBinaryPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.New(&hclog.LoggerOptions{Name: "plugin"}),
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to connect to agent plugin: %w", err)
	}

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

	fmt.Printf("[Manager] Agent plugin '%s' hot-reloaded\n", name)
	return nil
}

// hotReloadLLMLocked 內部重載 LLM（需已持有鎖）
func (m *Manager) hotReloadLLMLocked(name, newBinaryPath string) error {
	if err := m.unloadLLMLocked(name); err != nil {
		// 忽略未找到錯誤
	}

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]plugin.Plugin{
			"llm": &LLMGRPCPlugin{},
		},
		Cmd:              exec.Command(newBinaryPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.New(&hclog.LoggerOptions{Name: "llm-plugin"}),
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to connect to LLM plugin: %w", err)
	}

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

	fmt.Printf("[Manager] LLM plugin '%s' hot-reloaded\n", name)
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
		fmt.Printf("[Manager] Plugin '%s' killed\n", name)
	}
	m.clients = make(map[string]*plugin.Client)
	m.plugins = make(map[string]DSCPlugin)
	m.agents = make(map[string]Agent)
	m.llms = make(map[string]LLMProvider)
}