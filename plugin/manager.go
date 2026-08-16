package plugin

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"sync"

	"dsc/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
)

// Manager 插件管理器
type Manager struct {
	mu              sync.RWMutex
	clients         map[string]*plugin.Client // 插件名 -> 客戶端
	plugins         map[string]DSCPlugin      // 插件名 -> DSC業務接口
	agents          map[string]Agent          // 插件名 -> Agent接口
	llms            map[string]LLMProvider    // 插件名 -> LLMProvider接口
	typeMap         map[string]string         // 插件名 -> 類型 ("dsc", "agent", "llm")
	agentServiceIDs map[string]uint32         // agent name -> serviceID
	config          *ManagerConfig
}

type ManagerConfig struct {
	PluginDir string
	Handshake plugin.HandshakeConfig
}

func NewManager(cfg *ManagerConfig) *Manager {
	return &Manager{
		clients:         make(map[string]*plugin.Client),
		plugins:         make(map[string]DSCPlugin),
		agents:          make(map[string]Agent),
		llms:            make(map[string]LLMProvider),
		typeMap:         make(map[string]string),
		agentServiceIDs: make(map[string]uint32),
		config:          cfg,
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
	m.typeMap[name] = "dsc"

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
		delete(m.typeMap, name)
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
func (m *Manager) LoadAgent(name string, binaryPath string, serviceID uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if serviceID == 0 {
		return fmt.Errorf("serviceID must not be 0")
	}

	// 如果已加載，先卸載
	if _, exists := m.agents[name]; exists {
		m.unloadAgentLocked(name)
	}

	// 構建命令，加入 -llm-service-id 參數
	cmdArgs := []string{"-llm-service-id", strconv.FormatUint(uint64(serviceID), 10)}
	cmd := exec.Command(binaryPath, cmdArgs...)

	// 創建插件客戶端
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]plugin.Plugin{
			"agent": &AgentGRPCPlugin{},
		},
		Cmd:              cmd,
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
	m.typeMap[name] = "agent"

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
		delete(m.typeMap, name)
		delete(m.agentServiceIDs, name)
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

// LoadAgentAndGetBroker 加載 Agent 插件，並返回其 GRPCBroker 和 serviceID 以供宿主註冊服務。
// 該方法會將 Agent 納入 Manager 管理，支持後續熱重載。
func (m *Manager) LoadAgentAndGetBroker(name, binaryPath string) (*plugin.GRPCBroker, uint32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果已加載，先卸載
	if _, exists := m.agents[name]; exists {
		m.unloadAgentLocked(name)
	}

	// 創建插件客戶端（先不傳遞 serviceID，稍後通過 broker 生成）
	cmd := exec.Command(binaryPath)
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]plugin.Plugin{
			"agent": &AgentGRPCPlugin{},
		},
		Cmd:              cmd,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.New(&hclog.LoggerOptions{Name: "plugin"}),
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, 0, fmt.Errorf("failed to connect to agent plugin: %w", err)
	}

	grpcClient, ok := rpcClient.(*plugin.GRPCClient)
	if !ok {
		client.Kill()
		return nil, 0, fmt.Errorf("agent plugin is not a gRPC client")
	}

	broker := grpcClient.Broker()
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

	fmt.Printf("[Manager] Agent plugin '%s' loaded successfully, serviceID: %d\n", name, serviceID)
	return broker, serviceID, nil
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
	m.typeMap[name] = "llm"

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
		delete(m.typeMap, name)
		fmt.Printf("[Manager] LLM plugin '%s' unloaded\n", name)
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
	newClient := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]plugin.Plugin{
			"dsc_plugin": &DSCPluginGRPC{},
		},
		Cmd:              exec.Command(newBinaryPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.New(&hclog.LoggerOptions{Name: "plugin"}),
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

	fmt.Printf("[Manager] Plugin '%s' hot-reloaded\n", name)
	return nil
}

// hotReloadAgentLocked 內部重載 Agent（需已持有鎖）
func (m *Manager) hotReloadAgentLocked(name, newBinaryPath string) error {
	oldServiceID, ok := m.agentServiceIDs[name]
	if !ok {
		return fmt.Errorf("no serviceID recorded for agent %s", name)
	}

	// 1. 建立新 client 並驗證
	newClient := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]plugin.Plugin{
			"agent": &AgentGRPCPlugin{},
		},
		Cmd:              exec.Command(newBinaryPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.New(&hclog.LoggerOptions{Name: "plugin"}),
	})

	rpcClient, err := newClient.Client()
	if err != nil {
		newClient.Kill()
		return fmt.Errorf("new agent plugin connection failed: %w", err)
	}

	grpcClient, ok := rpcClient.(*plugin.GRPCClient)
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

	// 2. 卸載舊 client（若存在）
	if oldClient, exists := m.clients[name]; exists {
		oldClient.Kill()
	}
	// 3. 更新映射
	m.clients[name] = newClient
	m.agents[name] = newImpl
	m.typeMap[name] = "agent"

	fmt.Printf("[Manager] Agent plugin '%s' hot-reloaded, serviceID: %d\n", name, oldServiceID)
	return nil
}

// hotReloadLLMLocked 內部重載 LLM（需已持有鎖）
func (m *Manager) hotReloadLLMLocked(name, newBinaryPath string) error {
	// 1. 建立新 client 並驗證
	newClient := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: m.config.Handshake,
		Plugins: map[string]plugin.Plugin{
			"llm": &LLMGRPCPlugin{},
		},
		Cmd:              exec.Command(newBinaryPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger:           hclog.New(&hclog.LoggerOptions{Name: "llm-plugin"}),
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
	m.typeMap = make(map[string]string)
	m.agentServiceIDs = make(map[string]uint32)
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