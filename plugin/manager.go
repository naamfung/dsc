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
	plugins map[string]DSCPlugin      // 插件名 -> 業務接口
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

// HotReload 熱重載：卸載舊版本，加載新版本
func (m *Manager) HotReload(name string, newBinaryPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. 先卸載舊的
	if err := m.unloadLocked(name); err != nil {
		// 忽略未找到錯誤，繼續加載
	}

	// 2. 加載新的
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
		return err
	}

	raw, err := rpcClient.Dispense("dsc_plugin")
	if err != nil {
		client.Kill()
		return err
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
}